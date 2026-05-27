package main

import (
	"fmt"
	"os"

	"github.com/dmallubhotla/kestrel/internal/guard"
	"github.com/dmallubhotla/kestrel/internal/helm"
	"github.com/dmallubhotla/kestrel/internal/runner"
	"github.com/spf13/cobra"
)

var (
	tagOverride  string
	deployAll    bool
	targetFilter string
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Helm release operations (deploy, list, uninstall)",
}

var releaseDeployCmd = &cobra.Command{
	Use:   "deploy [release] [extra helm args...]",
	Short: "Deploy one or all helm releases",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireCI(); err != nil {
			return err
		}

		if deployAll {
			return ciDeployAllReleases(args)
		}

		if len(args) == 0 {
			return fmt.Errorf("release name required (or use --all)\navailable: %v", cfg.ReleaseNames())
		}

		return ciDeploySingleRelease(args[0], args[1:])
	},
}

var releaseListCmd = &cobra.Command{
	Use:   "ls [release]",
	Short: "List helm release info",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			for _, name := range cfg.ReleaseNames() {
				r := cfg.Helm.Releases[name]
				_, _ = fmt.Fprintf(os.Stdout, "%-20s %-30s target=%s\n", name, r.ReleaseName, r.Target)
			}
			return nil
		}

		releaseName := args[0]
		release, ok := cfg.Helm.Releases[releaseName]
		if !ok {
			return fmt.Errorf("release %q not found in .kestconfig\navailable: %v", releaseName, cfg.ReleaseNames())
		}

		resolved, err := resolveTargetForCI(release.Target)
		if err != nil {
			return err
		}
		return helm.List(cfg, release, resolved)
	},
}

var releaseUninstallCmd = &cobra.Command{
	Use:   "uninstall <release>",
	Short: "Remove a helm release",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		releaseName := args[0]
		release, ok := cfg.Helm.Releases[releaseName]
		if !ok {
			return fmt.Errorf("release %q not found in .kestconfig\navailable: %v", releaseName, cfg.ReleaseNames())
		}

		resolved, err := resolveTargetForCI(release.Target)
		if err != nil {
			return err
		}
		return helm.Uninstall(cfg, release, resolved)
	},
}

func ciDeploySingleRelease(releaseName string, extraArgs []string) error {
	release, ok := cfg.Helm.Releases[releaseName]
	if !ok {
		return fmt.Errorf("release %q not found in .kestconfig\navailable: %v", releaseName, cfg.ReleaseNames())
	}

	// Guards always enforced in CI — no --force bypass.
	if err := guard.CheckCleanWorktree(); err != nil {
		return err
	}
	if err := guard.CheckBranch(release.Target); err != nil {
		return err
	}

	resolved, err := resolveTargetForCI(release.Target)
	if err != nil {
		return err
	}

	// Deploy scripts
	for _, script := range cfg.EffectiveDeployScripts(release) {
		fmt.Fprintf(os.Stderr, "info: running deploy script %s\n", script)
		if err := runner.Run("bash", script); err != nil {
			return fmt.Errorf("deploy script %s failed: %w", script, err)
		}
	}

	tag, err := helm.ResolveTag(release.Target, tagOverride)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "info: deploying %s (release %s) to %s — tag %s\n",
		releaseName, release.ReleaseName, release.Target, tag)

	return helm.Deploy(cfg, release, resolved, tag, extraArgs)
}

func ciDeployAllReleases(extraArgs []string) error {
	names := cfg.ReleaseNames()
	if targetFilter != "" {
		names = cfg.ReleasesForTarget(targetFilter)
	}

	if len(names) == 0 {
		return fmt.Errorf("no releases found")
	}

	fmt.Fprintf(os.Stderr, "info: deploying %d release(s): %v\n", len(names), names)

	for _, name := range names {
		if err := ciDeploySingleRelease(name, extraArgs); err != nil {
			return fmt.Errorf("release %s: %w", name, err)
		}
	}
	return nil
}

func init() {
	releaseDeployCmd.Flags().StringVarP(&tagOverride, "tag", "t", "", "override the image tag")
	releaseDeployCmd.Flags().BoolVar(&deployAll, "all", false, "deploy all configured releases")
	releaseDeployCmd.Flags().StringVar(&targetFilter, "target", "", "filter releases by target (used with --all)")

	releaseCmd.AddCommand(releaseDeployCmd)
	releaseCmd.AddCommand(releaseListCmd)
	releaseCmd.AddCommand(releaseUninstallCmd)
	rootCmd.AddCommand(releaseCmd)
}
