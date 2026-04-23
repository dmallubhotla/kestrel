package main

import (
	"fmt"
	"os"

	"github.com/example/kestrel/internal/guard"
	"github.com/example/kestrel/internal/helm"
	"github.com/example/kestrel/internal/runner"
	"github.com/spf13/cobra"
)

var tagOverride string

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Helm release operations (deploy, list, uninstall)",
}

var releaseDeployCmd = &cobra.Command{
	Use:   "deploy [extra helm args...]",
	Short: "Deploy the helm chart to a target",
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("--environment (-e) is required")
		}

		if err := requireCI(); err != nil {
			return err
		}

		// Guards always enforced in CI — no --force bypass.
		if err := guard.CheckCleanWorktree(); err != nil {
			return err
		}
		if err := guard.CheckBranch(environment); err != nil {
			return err
		}

		resolved, err := resolveTargetForCI(environment)
		if err != nil {
			return err
		}

		// Run deploy scripts before deploy.
		for _, script := range cfg.Helm.DeployScripts {
			fmt.Fprintf(os.Stderr, "info: running deploy script %s\n", script)
			if err := runner.Run("bash", script); err != nil {
				return fmt.Errorf("deploy script %s failed: %w", script, err)
			}
		}

		tag, err := helm.ResolveTag(environment, tagOverride)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "info: deploying container version: %s\n", tag)

		return helm.Deploy(cfg, environment, resolved, tag, args)
	},
}

var releaseListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List deployment info for the release",
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("--environment (-e) is required")
		}
		resolved, err := resolveTargetForCI(environment)
		if err != nil {
			return err
		}
		return helm.List(cfg, resolved)
	},
}

var releaseUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the helm release",
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("--environment (-e) is required")
		}
		resolved, err := resolveTargetForCI(environment)
		if err != nil {
			return err
		}
		return helm.Uninstall(cfg, resolved)
	},
}

func init() {
	releaseDeployCmd.Flags().StringVarP(&tagOverride, "tag", "t", "", "override the image tag")

	releaseCmd.AddCommand(releaseDeployCmd)
	releaseCmd.AddCommand(releaseListCmd)
	releaseCmd.AddCommand(releaseUninstallCmd)
	rootCmd.AddCommand(releaseCmd)
}
