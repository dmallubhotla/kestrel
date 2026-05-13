//go:build legacy

// Package cmd's terraform.go is the legacy `kest terraform` proxy that
// shells terraform commands into <iac_dir>/live/<env>/. It pre-dates
// `kest swoop`, which now handles all terraform workflows. The file is
// excluded from default builds via the `legacy` build tag; build with
// `-tags legacy` to revive it.
package cmd

import (
	"github.com/example/kestrel/internal/terraform"
	"github.com/spf13/cobra"
)

var terraformCmd = &cobra.Command{
	Use:     "terraform [subcommand] [flags]",
	Short:   "Run terraform commands in the appropriate target directory",
	GroupID: "deploy",
	Long:    "Proxies terraform commands into the correct misc/iac/live/<target>/ directory.",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return cmd.Help()
		}
		resolved, err := cfg.ResolveTarget(environment)
		if err != nil {
			return err
		}
		if err := ensureSSOSession(resolved.AwsProfile); err != nil {
			return err
		}
		return terraform.Run(cfg, environment, resolved, args)
	},
	// Don't parse flags after the first positional arg — pass them to terraform.
	DisableFlagParsing: false,
}

func init() {
	rootCmd.AddCommand(terraformCmd)
}
