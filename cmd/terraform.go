package cmd

import (
	"github.com/example/kestrel/internal/terraform"
	"github.com/spf13/cobra"
)

var terraformCmd = &cobra.Command{
	Use:     "terraform [subcommand] [flags]",
	Short:   "Run terraform commands in the appropriate environment directory",
	GroupID: "deploy",
	Long:    "Proxies terraform commands into the correct misc/iac/live/<env>/ directory.",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return cmd.Help()
		}
		envCfg, err := cfg.ResolveEnv(environment)
		if err != nil {
			return err
		}
		return terraform.Run(cfg, environment, envCfg, args)
	},
	// Don't parse flags after the first positional arg — pass them to terraform.
	DisableFlagParsing: false,
}

func init() {
	rootCmd.AddCommand(terraformCmd)
}
