package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/example/kestrel/internal/swoop"
	"github.com/spf13/cobra"
)

var swoopProfile string

var swoopCmd = &cobra.Command{
	Use:     "swoop",
	Short:   "Discover and operate on terraform roots",
	GroupID: "deploy",
	Long: `Find, list, and run terraform commands across all roots in a project.

Running without a subcommand opens an interactive picker to browse and
select a terraform root. Use arrow keys or type to filter, then press
enter to plan, ctrl+a to apply, or ctrl+i to init.`,
	RunE: runSwoopInteractive,
}

var swoopListCmd = &cobra.Command{
	Use:   "list [target]",
	Short: "List discovered terraform roots",
	Long: `Discover and display all terraform roots in the project.

Optionally provide a target to filter results (exact path, glob, or substring).
Use --profile to filter by account profile directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir, err := resolveBaseDir()
		if err != nil {
			return err
		}

		roots, err := discoverRoots(baseDir)
		if err != nil {
			return err
		}

		if len(roots) == 0 {
			fmt.Fprintf(os.Stderr, "No terraform roots found under %s\n", baseDir)
			return nil
		}

		// Apply filters.
		if swoopProfile != "" {
			roots = swoop.ResolveByProfile(roots, swoopProfile)
		}
		if len(args) > 0 {
			roots = swoop.Resolve(roots, args[0])
		}

		if len(roots) == 0 {
			fmt.Fprintln(os.Stderr, "No matching roots found.")
			return nil
		}

		// Load local state for recency info.
		state, _ := swoop.LoadState(baseDir)

		// Sort: roots with recent activity first, then alphabetical.
		sortRoots(roots, state)

		printRootTable(roots, state)
		return nil
	},
}

// resolveBaseDir determines the directory to scan for terraform roots.
// It uses terraform.iac_dir from config if set, otherwise the project root
// (directory containing .kestconfig), otherwise cwd.
func resolveBaseDir() (string, error) {
	// If terraform.iac_dir is configured, use it relative to the project config location.
	if cfg != nil && cfg.Terraform.IACDir != "" {
		if cfg.Sources.Project != "" {
			projectRoot := filepath.Dir(cfg.Sources.Project)
			return filepath.Abs(filepath.Join(projectRoot, cfg.Terraform.IACDir))
		}
		return filepath.Abs(cfg.Terraform.IACDir)
	}

	// If a .kestconfig exists, use its directory as the base.
	if cfg != nil && cfg.Sources.Project != "" {
		return filepath.Abs(filepath.Dir(cfg.Sources.Project))
	}

	// Fall back to cwd.
	return os.Getwd()
}

// discoverRoots discovers terraform roots and enriches them with account IDs.
func discoverRoots(baseDir string) ([]swoop.Root, error) {
	roots, err := swoop.Discover(baseDir)
	if err != nil {
		return nil, fmt.Errorf("discovering roots: %w", err)
	}
	if len(roots) > 0 {
		swoop.EnrichWithAccountIDs(roots, baseDir)
	}
	return roots, nil
}

func sortRoots(roots []swoop.Root, state *swoop.State) {
	// Alphabetical-only when configured.
	if cfg != nil && cfg.Swoop.SortOrder == "alpha" {
		sort.Slice(roots, func(i, j int) bool {
			return roots[i].Path < roots[j].Path
		})
		return
	}

	// Default: recency-first ordering.
	sort.Slice(roots, func(i, j int) bool {
		ti := lastActivity(state, roots[i].Path)
		tj := lastActivity(state, roots[j].Path)

		// Roots with activity sort before those without.
		if !ti.IsZero() && tj.IsZero() {
			return true
		}
		if ti.IsZero() && !tj.IsZero() {
			return false
		}
		// Both have activity: most recent first.
		if !ti.IsZero() && !tj.IsZero() && !ti.Equal(tj) {
			return ti.After(tj)
		}
		// Fall back to alphabetical.
		return roots[i].Path < roots[j].Path
	})
}

func printRootTable(roots []swoop.Root, state *swoop.State) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ROOT\tPROFILE\tAWS PROFILE\tTF VERSION\tINIT\tLAST ACTIVITY")

	for _, r := range roots {
		init := "-"
		if r.Initialized {
			init = "yes"
		}

		ver := r.TFVersion
		if ver == "" {
			ver = "-"
		}

		activity := lastActivityStr(state, r.Path)

		aws := swoop.ResolveAWSProfile(r, cfg, environment)
		if aws == "" {
			aws = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Path, r.Profile, aws, ver, init, activity)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d root(s) found\n", len(roots))
}

func runSwoopInteractive(cmd *cobra.Command, args []string) error {
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

	state, _ := swoop.LoadState(baseDir)
	sortRoots(roots, state)

	m := newSwoopTUI(roots, state)
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return err
	}

	action := result.(swoopTUIModel).result
	if action.action == "" {
		return nil // cancelled
	}

	fmt.Fprintf(os.Stderr, "\n%s\n\n",
		swoopHelpStyle.Render(
			fmt.Sprintf("hint: kest swoop %s %s", action.action, action.root.Path)))
	return runSwoopAction(action.action, action.root.Path)
}

func init() {
	swoopListCmd.Flags().StringVar(&swoopProfile, "profile", "", "filter by account profile directory")
	swoopListCmd.ValidArgsFunction = completeSwoopRoots
	swoopListCmd.RegisterFlagCompletionFunc("profile", completeSwoopProfiles)

	swoopCmd.AddCommand(swoopListCmd)
	rootCmd.AddCommand(swoopCmd)
}
