package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/example/kestrel/internal/guard"
	"github.com/example/kestrel/internal/swoop"
	"github.com/spf13/cobra"
)

var (
	swoopChanged       string
	swoopActionProfile string
)

var swoopInitCmd = &cobra.Command{
	Use:   "init [target]",
	Short: "Run terraform init in target root(s)",
	Long: `Initialize terraform root directories.

Target can be an exact path, glob pattern, or substring match.
If the target matches a single root, it runs immediately.
If it matches multiple roots (via glob), they run sequentially.

Use --changed to target roots with modified .tf files.
Use --profile to filter by account profile directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return runSwoopActionCmd("init", target)
	},
}

var swoopPlanCmd = &cobra.Command{
	Use:   "plan [target]",
	Short: "Run terraform plan in target root(s)",
	Long: `Plan changes for terraform root directories.

Target can be an exact path, glob pattern, or substring match.
If the target matches a single root, it runs immediately.
If it matches multiple roots (via glob), they run sequentially with a summary.

Use --changed to target roots with modified .tf files.
Use --profile to filter by account profile directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return runSwoopActionCmd("plan", target)
	},
}

var swoopApplyCmd = &cobra.Command{
	Use:   "apply [target]",
	Short: "Run terraform apply in target root(s)",
	Long: `Apply changes for terraform root directories.

Target can be an exact path, glob pattern, or substring match.
If the target matches a single root, it runs immediately.
If it matches multiple roots (via glob), they run sequentially with a summary.

Use --changed to target roots with modified .tf files.
Use --profile to filter by account profile directory.

Apply has additional safety guards:
- Requires --force when not in CI
- Checks for a clean git worktree`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return runSwoopActionCmd("apply", target)
	},
}

// runSwoopActionCmd resolves targets (including --changed and --profile flags)
// and dispatches to single or batch execution.
func runSwoopActionCmd(action, target string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return err
	}

	roots, err := discoverRoots(baseDir)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return fmt.Errorf("no terraform roots found under %s", baseDir)
	}

	matches, matchType, err := resolveTargets(roots, baseDir, target)
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return fmt.Errorf("no matching roots found")
	}

	// Fuzzy matches require user confirmation.
	if matchType == swoop.MatchFuzzy {
		if !confirmFuzzyMatch(matches, target) {
			return fmt.Errorf("aborted")
		}
	}

	// Single root — use the direct path.
	if len(matches) == 1 {
		return runSwoopAction(action, matches[0].Path)
	}

	// Multiple roots — batch mode.
	// For non-glob substring matches that are ambiguous, error out.
	if !isBatchTarget(target) && swoopChanged == "" && swoopActionProfile == "" {
		return ambiguousTargetError(matches, target)
	}

	return runSwoopBatch(action, matches, baseDir)
}

// resolveTargets combines target string, --changed, and --profile into a set of roots.
// Returns the matched roots and the match type used for the target resolution.
func resolveTargets(roots []swoop.Root, baseDir, target string) ([]swoop.Root, swoop.MatchType, error) {
	matches := roots

	// Filter by profile first.
	if swoopActionProfile != "" {
		matches = swoop.ResolveByProfile(matches, swoopActionProfile)
	}

	// Filter by --changed.
	if swoopChanged != "" {
		ref := ""
		if swoopChanged != "true" {
			ref = swoopChanged
		}
		changed, err := swoop.ChangedRoots(matches, baseDir, ref)
		if err != nil {
			return nil, swoop.MatchAll, fmt.Errorf("detecting changed roots: %w", err)
		}
		matches = changed
	}

	// Filter by target string.
	matchType := swoop.MatchAll
	if target != "" {
		matches, matchType = swoop.ResolveWithType(matches, target)
	}

	return matches, matchType, nil
}

// isBatchTarget returns true if the target looks like a glob pattern
// (contains * or ?) indicating the user intended multi-root matching.
func isBatchTarget(target string) bool {
	return strings.ContainsAny(target, "*?[")
}

// runSwoopAction executes a terraform action against a single root.
func runSwoopAction(action, target string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return err
	}

	roots, err := discoverRoots(baseDir)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return fmt.Errorf("no terraform roots found under %s", baseDir)
	}

	matches, matchType := swoop.ResolveWithType(roots, target)
	if len(matches) == 0 {
		return fmt.Errorf("no roots matching %q", target)
	}
	if len(matches) > 1 {
		return ambiguousTargetError(matches, target)
	}

	if matchType == swoop.MatchFuzzy {
		if !confirmFuzzyMatch(matches, target) {
			return fmt.Errorf("aborted")
		}
	}

	root := matches[0]
	return executeSingle(action, root, baseDir)
}

func executeSingle(action string, root swoop.Root, baseDir string) error {
	// Apply-specific guards.
	if action == "apply" {
		if err := applyGuards(); err != nil {
			return err
		}
	}

	// Resolve AWS_PROFILE.
	awsProfile := swoop.ResolveAWSProfile(root, cfg, environment)

	if err := ensureSSOSession(awsProfile); err != nil {
		return err
	}

	// tfenv preflight check.
	if err := handleTFVersionCheck(root); err != nil {
		return err
	}

	// Print context.
	fmt.Fprintf(os.Stderr, "root:    %s\n", root.Path)
	fmt.Fprintf(os.Stderr, "profile: %s\n", root.Profile)
	if awsProfile != "" {
		fmt.Fprintf(os.Stderr, "aws:     %s\n", awsProfile)
	}
	if root.TFVersion != "" {
		fmt.Fprintf(os.Stderr, "tf:      %s\n", root.TFVersion)
	}
	fmt.Fprintln(os.Stderr)

	// Execute.
	result, err := swoop.RunTerraform(root, awsProfile, action)

	// Record to local state regardless of error.
	recordAction(baseDir, root.Path, action, result)

	if err != nil {
		return fmt.Errorf("terraform %s failed: %w", action, err)
	}
	return nil
}

// handleTFVersionCheck checks the terraform version and offers to install via
// tfenv if there's a mismatch.
func handleTFVersionCheck(root swoop.Root) error {
	check := swoop.CheckTFVersion(root)
	if check.OK {
		return nil
	}

	if check.Installed != "" {
		fmt.Fprintf(os.Stderr, "warning: root requires terraform %s (from .terraform-version) but %s is active\n",
			check.Required, check.Installed)
	} else {
		fmt.Fprintf(os.Stderr, "warning: root requires terraform %s (from .terraform-version)\n",
			check.Required)
	}

	if !check.TfenvAvailable {
		fmt.Fprintf(os.Stderr, "  Install manually or use tfenv: tfenv install %s\n", check.Required)
		return nil // warn but don't block
	}

	installCmd := swoop.FormatTFVersionCommand(check.Required)
	fmt.Fprintf(os.Stderr, "\n  %s\n\n", installCmd)
	fmt.Fprintf(os.Stderr, "Install now? [y/N] ")

	var answer string
	fmt.Scanln(&answer)
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Fprintln(os.Stderr, "Continuing with current terraform version.")
		return nil
	}

	fmt.Fprintln(os.Stderr)
	if err := swoop.InstallTFVersion(check.Required); err != nil {
		return fmt.Errorf("tfenv install failed: %w", err)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

// batchResult tracks the outcome of one root in a batch run.
type batchResult struct {
	root    swoop.Root
	err     error
	summary string
}

func runSwoopBatch(action string, roots []swoop.Root, baseDir string) error {
	// Apply-specific guards (once, not per root).
	if action == "apply" {
		if err := applyGuards(); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "Running terraform %s on %d root(s)...\n\n", action, len(roots))

	// Check SSO sessions once per unique profile before starting the batch.
	checkedProfiles := map[string]bool{}
	for _, root := range roots {
		p := swoop.ResolveAWSProfile(root, cfg, environment)
		if p != "" && !checkedProfiles[p] {
			if err := ensureSSOSession(p); err != nil {
				return err
			}
			checkedProfiles[p] = true
		}
	}

	results := make([]batchResult, len(roots))

	for i, root := range roots {
		awsProfile := swoop.ResolveAWSProfile(root, cfg, environment)

		// Print header.
		fmt.Fprintf(os.Stderr, "━━━ [%d/%d] %s ━━━\n", i+1, len(roots), root.Path)

		check := swoop.CheckTFVersion(root)
		if !check.OK {
			fmt.Fprintf(os.Stderr, "  warning: requires terraform %s but %s is active\n", check.Required, check.Installed)
		}

		execResult, err := swoop.RunTerraform(root, awsProfile, action)
		recordAction(baseDir, root.Path, action, execResult)

		br := batchResult{root: root, err: err}
		if execResult != nil {
			br.summary = execResult.PlanSummary
		}
		results[i] = br

		if err != nil {
			fmt.Fprintf(os.Stderr, "  FAILED: %v\n", err)
		}
		fmt.Fprintln(os.Stderr)
	}

	// Print summary.
	printBatchSummary(action, results)

	// Return error if any root failed.
	var failed int
	for _, r := range results {
		if r.err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d root(s) failed", failed, len(results))
	}
	return nil
}

func printBatchSummary(action string, results []batchResult) {
	fmt.Fprintf(os.Stderr, "━━━ Swoop %s summary ━━━\n", action)
	w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
	for _, r := range results {
		status := "OK"
		if r.err != nil {
			status = "FAILED"
		}
		detail := ""
		if r.summary != "" {
			detail = r.summary
		}
		if detail != "" {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", r.root.Path, status, detail)
		} else {
			fmt.Fprintf(w, "  %s\t%s\n", r.root.Path, status)
		}
	}
	w.Flush()
}

func applyGuards() error {
	if force {
		return nil
	}
	if err := guard.CheckCI(); err != nil {
		return err
	}
	if err := guard.CheckCleanWorktree(); err != nil {
		return err
	}
	return nil
}

// confirmFuzzyMatch prints the resolved root(s) and prompts the user for
// confirmation. Returns true if the user confirms.
func confirmFuzzyMatch(matches []swoop.Root, target string) bool {
	fmt.Fprintf(os.Stderr, "fuzzy match for %q resolved to:\n", target)
	for _, m := range matches {
		fmt.Fprintf(os.Stderr, "  %s\n", m.Path)
	}
	fmt.Fprintf(os.Stderr, "\nProceed? [y/N] ")

	var answer string
	fmt.Scanln(&answer)
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func ambiguousTargetError(matches []swoop.Root, target string) error {
	msg := fmt.Sprintf("target %q matches %d roots — be more specific or use a glob pattern:\n", target, len(matches))
	for _, m := range matches {
		msg += fmt.Sprintf("  %s\n", m.Path)
	}
	return fmt.Errorf("%s", msg)
}

func recordAction(baseDir, rootPath, action string, result *swoop.ExecResult) {
	state, err := swoop.LoadState(baseDir)
	if err != nil {
		return
	}

	switch action {
	case "init":
		state.RecordInit(rootPath)
	case "plan":
		summary := ""
		if result != nil {
			summary = result.PlanSummary
		}
		state.RecordPlan(rootPath, summary)
	case "apply":
		state.RecordApply(rootPath)
	}

	state.Save()
}

func init() {
	// Shared flags for all action commands.
	for _, c := range []*cobra.Command{swoopInitCmd, swoopPlanCmd, swoopApplyCmd} {
		c.Flags().StringVar(&swoopChanged, "changed", "", "target roots with modified .tf files (optionally specify git ref, e.g. --changed=HEAD~3)")
		// Allow --changed without a value (defaults to "true" meaning "use merge-base").
		c.Flags().Lookup("changed").NoOptDefVal = "true"
		c.Flags().StringVar(&swoopActionProfile, "profile", "", "filter by account profile directory")

		c.ValidArgsFunction = completeSwoopRoots
		c.RegisterFlagCompletionFunc("profile", completeSwoopProfiles)
	}

	swoopCmd.AddCommand(swoopInitCmd)
	swoopCmd.AddCommand(swoopPlanCmd)
	swoopCmd.AddCommand(swoopApplyCmd)
}
