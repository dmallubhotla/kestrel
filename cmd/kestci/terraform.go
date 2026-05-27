package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmallubhotla/kestrel/internal/guard"
	"github.com/dmallubhotla/kestrel/internal/resolve"
	"github.com/dmallubhotla/kestrel/internal/swoop"
	"github.com/spf13/cobra"
)

var tfInitCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Run terraform init in a root",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTerraformAction("init", args[0])
	},
}

var tfPlanCmd = &cobra.Command{
	Use:   "plan <path>",
	Short: "Run terraform plan in a root",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTerraformAction("plan", args[0])
	},
}

var tfApplyCmd = &cobra.Command{
	Use:   "apply <path>",
	Short: "Run terraform apply in a root",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTerraformAction("apply", args[0])
	},
}

func runTerraformAction(action, target string) error {
	if err := requireCI(); err != nil {
		return err
	}

	// Apply-specific guards (always enforced, no --force).
	if action == "apply" {
		if err := guard.CheckCleanWorktree(); err != nil {
			return err
		}
	}

	baseDir, err := ciResolveBaseDir()
	if err != nil {
		return err
	}

	roots, err := swoop.Discover(baseDir)
	if err != nil {
		return fmt.Errorf("discovering roots: %w", err)
	}
	if len(roots) == 0 {
		return fmt.Errorf("no terraform roots found under %s", baseDir)
	}
	swoop.EnrichWithAccountIDs(roots, baseDir)

	// Resolve target — exact path or glob only, no fuzzy matching.
	matches, matchType := swoop.ResolveWithType(roots, target)
	if len(matches) == 0 {
		return fmt.Errorf("no roots matching %q", target)
	}
	if matchType == swoop.MatchFuzzy {
		return fmt.Errorf("ambiguous target %q — use an exact path or glob in CI", target)
	}

	// Execute each matched root.
	if len(matches) == 1 {
		return executeCIAction(action, matches[0], baseDir)
	}

	// Batch mode for globs.
	fmt.Fprintf(os.Stderr, "Running terraform %s on %d root(s)...\n\n", action, len(matches))
	var failed int
	for i, root := range matches {
		fmt.Fprintf(os.Stderr, "━━━ [%d/%d] %s ━━━\n", i+1, len(matches), root.Path)
		if err := executeCIAction(action, root, baseDir); err != nil {
			fmt.Fprintf(os.Stderr, "  FAILED: %v\n", err)
			failed++
		}
		fmt.Fprintln(os.Stderr)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d root(s) failed", failed, len(matches))
	}
	return nil
}

func executeCIAction(action string, root swoop.Root, baseDir string) error {
	// Resolve AWS profile — CI uses ambient credentials, so this may be
	// empty. That's fine; terraform will use env vars directly.
	awsProfile := resolve.AWSProfileForRoot(cfg, root.Dir, root.AccountID, environment)

	fmt.Fprintf(os.Stderr, "root:    %s\n", root.Path)
	if awsProfile != "" {
		fmt.Fprintf(os.Stderr, "aws:     %s\n", awsProfile)
	}
	if root.TFVersion != "" {
		fmt.Fprintf(os.Stderr, "tf:      %s\n", root.TFVersion)
	}
	fmt.Fprintln(os.Stderr)

	result, err := swoop.RunTerraform(root, awsProfile, action)

	// Record to local state.
	state, stateErr := swoop.LoadState(baseDir)
	if stateErr == nil && result != nil {
		switch action {
		case "init":
			state.RecordInit(root.Path)
		case "plan":
			state.RecordPlan(root.Path, result.PlanSummary)
		case "apply":
			state.RecordApply(root.Path)
		}
		_ = state.Save()
	}

	if err != nil {
		return fmt.Errorf("terraform %s failed: %w", action, err)
	}
	return nil
}

// ciResolveBaseDir determines the terraform discovery base directory.
func ciResolveBaseDir() (string, error) {
	if cfg != nil && cfg.Terraform.IACDir != "" {
		if cfg.Sources.Project != "" {
			projectRoot := filepath.Dir(cfg.Sources.Project)
			return filepath.Abs(filepath.Join(projectRoot, cfg.Terraform.IACDir))
		}
		return filepath.Abs(cfg.Terraform.IACDir)
	}
	if cfg != nil && cfg.Sources.Project != "" {
		return filepath.Abs(filepath.Dir(cfg.Sources.Project))
	}
	return os.Getwd()
}

func init() {
	rootCmd.AddCommand(tfInitCmd)
	rootCmd.AddCommand(tfPlanCmd)
	rootCmd.AddCommand(tfApplyCmd)
}
