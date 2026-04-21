package cmd

import (
	"fmt"
	"os"

	"github.com/example/kestrel/internal/guard"
	"github.com/example/kestrel/internal/helm"
	"github.com/example/kestrel/internal/runner"
	"github.com/spf13/cobra"
)

var (
	tagOverride string
)

var helmCmd = &cobra.Command{
	Use:     "release",
	Aliases: []string{"helm"},
	Short:   "Helm release operations (deploy, list, uninstall)",
	GroupID: "deploy",
}

var helmDeployCmd = &cobra.Command{
	Use:   "deploy [extra helm args...]",
	Short: "Deploy the helm chart to a target",
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("--environment (-e) is required")
		}

		resolved, err := cfg.ResolveTarget(environment)
		if err != nil {
			return err
		}

		if err := ensureSSOSession(resolved.AwsProfile); err != nil {
			return err
		}

		// Safety checks
		if !force {
			if err := guard.CheckCI(); err != nil {
				return err
			}
			if err := guard.CheckCleanWorktree(); err != nil {
				return err
			}
			if err := guard.CheckBranch(environment); err != nil {
				return err
			}
		}

		// Run deploy scripts before deploy
		for _, script := range cfg.Helm.DeployScripts {
			fmt.Fprintf(os.Stderr, "info: running deploy script %s\n", script)
			if err := runner.Run("bash", script); err != nil {
				return fmt.Errorf("deploy script %s failed: %w", script, err)
			}
		}

		// Resolve image tag
		tag, err := helm.ResolveTag(environment, tagOverride)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "info: deploying container version: %s\n", tag)

		return helm.Deploy(cfg, environment, resolved, tag, args)
	},
}

var helmListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List deployment info for the release",
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("--environment (-e) is required")
		}
		resolved, err := cfg.ResolveTarget(environment)
		if err != nil {
			return err
		}
		if err := ensureSSOSession(resolved.AwsProfile); err != nil {
			return err
		}
		return helm.List(cfg, resolved)
	},
}

var helmUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the helm release",
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("--environment (-e) is required")
		}
		resolved, err := cfg.ResolveTarget(environment)
		if err != nil {
			return err
		}
		if err := ensureSSOSession(resolved.AwsProfile); err != nil {
			return err
		}
		return helm.Uninstall(cfg, resolved)
	},
}

func init() {
	helmDeployCmd.Flags().StringVarP(&tagOverride, "tag", "t", "", "override the image tag")

	helmCmd.AddCommand(helmDeployCmd)
	helmCmd.AddCommand(helmListCmd)
	helmCmd.AddCommand(helmUninstallCmd)
	rootCmd.AddCommand(helmCmd)
}
