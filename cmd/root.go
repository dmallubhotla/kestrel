package cmd

import (
	"fmt"
	"os"

	"github.com/example/kestrel/internal/config"
	"github.com/spf13/cobra"
)

var (
	environment string
	verbose     bool
	cfg         *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "kest",
	Short: "Kestrel — unified helm & terraform orchestration",
	Long:  "A CLI that wraps Helm and Terraform workflows per-project, driven by .kestconfig files.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return err
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "debug: config loaded (chart=%s, iac_dir=%s)\n",
				cfg.Helm.Chart, cfg.Terraform.IACDir)
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&environment, "environment", "e", "", "target environment (dev, stage, prod)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose/debug output")
}
