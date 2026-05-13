package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/example/kestrel/internal/guard"
	"github.com/example/kestrel/internal/helm"
	"github.com/example/kestrel/internal/runner"
	"github.com/spf13/cobra"
)

var (
	tagOverride  string
	deployAll    bool
	targetFilter string
)

var helmCmd = &cobra.Command{
	Use:     "release",
	Aliases: []string{"helm"},
	Short:   "Helm release operations (deploy, list, uninstall)",
	GroupID: "deploy",
}

var helmDeployCmd = &cobra.Command{
	Use:   "deploy [release] [extra helm args...]",
	Short: "Deploy one or all helm releases",
	Long: `Deploy a helm release defined in .kestconfig. The release's target
determines which cluster and AWS account to use — no -e flag needed.

Use --all to deploy every release (optionally filtered by --target).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if deployAll {
			return deployAllReleases(args)
		}

		if len(args) == 0 {
			return fmt.Errorf("release name required (or use --all)\navailable: %v", cfg.ReleaseNames())
		}

		releaseName := args[0]
		extraArgs := args[1:]
		return deploySingleRelease(releaseName, extraArgs)
	},
	ValidArgsFunction: completeReleaseNames,
}

var helmListCmd = &cobra.Command{
	Use:   "ls [release]",
	Short: "List helm release info",
	Long: `Without arguments, lists all configured releases.
With a release name, queries helm for that release's status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// Show configured releases
			for _, name := range cfg.ReleaseNames() {
				r := cfg.Helm.Releases[name]
				fmt.Fprintf(os.Stdout, "%-20s %-30s target=%s\n", name, r.ReleaseName, r.Target)
			}
			return nil
		}

		releaseName := args[0]
		release, ok := cfg.Helm.Releases[releaseName]
		if !ok {
			return fmt.Errorf("release %q not found in .kestconfig\navailable: %v", releaseName, cfg.ReleaseNames())
		}

		resolved, err := cfg.ResolveTarget(release.Target)
		if err != nil {
			return err
		}
		if err := ensureSSOSession(resolved.AwsProfile); err != nil {
			return err
		}
		return helm.List(cfg, release, resolved)
	},
	ValidArgsFunction: completeReleaseNames,
}

var helmUninstallCmd = &cobra.Command{
	Use:   "uninstall <release>",
	Short: "Remove a helm release",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		releaseName := args[0]
		release, ok := cfg.Helm.Releases[releaseName]
		if !ok {
			return fmt.Errorf("release %q not found in .kestconfig\navailable: %v", releaseName, cfg.ReleaseNames())
		}

		resolved, err := cfg.ResolveTarget(release.Target)
		if err != nil {
			return err
		}
		if err := ensureSSOSession(resolved.AwsProfile); err != nil {
			return err
		}
		return helm.Uninstall(cfg, release, resolved)
	},
	ValidArgsFunction: completeReleaseNames,
}

func deploySingleRelease(releaseName string, extraArgs []string) error {
	release, ok := cfg.Helm.Releases[releaseName]
	if !ok {
		return fmt.Errorf("release %q not found in .kestconfig\navailable: %v", releaseName, cfg.ReleaseNames())
	}

	resolved, err := cfg.ResolveTarget(release.Target)
	if err != nil {
		return err
	}

	if err := ensureSSOSession(resolved.AwsProfile); err != nil {
		return err
	}

	// Safety checks (target from release config)
	if !force {
		if err := guard.CheckCI(); err != nil {
			return err
		}
		if err := guard.CheckCleanWorktree(); err != nil {
			return err
		}
		if err := guard.CheckBranch(release.Target); err != nil {
			return err
		}
	}

	// Deploy scripts
	for _, script := range cfg.EffectiveDeployScripts(release) {
		slog.Info("running deploy script", "release", releaseName, "script", script)
		fmt.Fprintf(os.Stderr, "info: running deploy script %s\n", script)
		if err := runner.Run("bash", script); err != nil {
			return fmt.Errorf("deploy script %s failed: %w", script, err)
		}
	}

	// Resolve image tag (uses target name for prod vs non-prod logic)
	tag, err := helm.ResolveTag(release.Target, tagOverride)
	if err != nil {
		return err
	}
	slog.Info("deploying release",
		"release", releaseName,
		"helm_release", release.ReleaseName,
		"target", release.Target,
		"tag", tag,
	)
	fmt.Fprintf(os.Stderr, "info: deploying %s (release %s) to %s — tag %s\n",
		releaseName, release.ReleaseName, release.Target, tag)

	return helm.Deploy(cfg, release, resolved, tag, extraArgs)
}

func deployAllReleases(extraArgs []string) error {
	names := cfg.ReleaseNames()
	if targetFilter != "" {
		names = cfg.ReleasesForTarget(targetFilter)
	}

	if len(names) == 0 {
		return fmt.Errorf("no releases found")
	}

	slog.Info("batch deploying releases", "count", len(names), "releases", names)
	fmt.Fprintf(os.Stderr, "info: deploying %d release(s): %v\n", len(names), names)

	for _, name := range names {
		if err := deploySingleRelease(name, extraArgs); err != nil {
			return fmt.Errorf("release %s: %w", name, err)
		}
	}
	return nil
}

func init() {
	helmDeployCmd.Flags().StringVarP(&tagOverride, "tag", "t", "", "override the image tag")
	helmDeployCmd.Flags().BoolVar(&deployAll, "all", false, "deploy all configured releases")
	helmDeployCmd.Flags().StringVar(&targetFilter, "target", "", "filter releases by target (used with --all)")

	helmCmd.AddCommand(helmDeployCmd)
	helmCmd.AddCommand(helmListCmd)
	helmCmd.AddCommand(helmUninstallCmd)
	rootCmd.AddCommand(helmCmd)
}
