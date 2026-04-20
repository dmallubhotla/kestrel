package cmd

import (
	"fmt"
	"os"

	"github.com/example/kestrel/internal/awslogin"
	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/guard"
	"github.com/example/kestrel/internal/logging"
	"github.com/example/kestrel/internal/profile"
	"github.com/spf13/cobra"
)

var (
	environment string
	verbose     bool
	cfg         *config.Config
	logCleanup  func()
)

var rootCmd = &cobra.Command{
	Use:   "kest",
	Short: "Kestrel — unified helm & terraform orchestration",
	Long:  "A CLI that wraps Helm and Terraform workflows per-project, driven by .kestconfig files.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Set up file logging (non-fatal if it fails)
		cleanup, err := logging.Init()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not initialize logging: %v\n", err)
		} else {
			logCleanup = cleanup
		}

		cfg, err = config.Load()
		if err != nil {
			return err
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "debug: config loaded (chart=%s, iac_dir=%s)\n",
				cfg.Helm.Chart, cfg.Terraform.IACDir)
		}

		// Fall back to active profile when no -e flag (skip in CI)
		if environment == "" && !guard.IsCI() {
			active, err := profile.Read()
			if err == nil && active != "" {
				environment = active
				if verbose {
					fmt.Fprintf(os.Stderr, "debug: using active profile %q\n", active)
				}
			}
		}

		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if logCleanup != nil {
			logCleanup()
		}
	},
}

// ensureSSOSession checks the AWS session for the given profile and runs
// sso login if needed. No-op when auto_sso_login is off or running in CI.
func ensureSSOSession(profile string) error {
	if cfg == nil || !cfg.AutoSSOLogin || guard.IsCI() {
		return nil
	}
	return awslogin.EnsureSession(profile)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddGroup(
		&cobra.Group{ID: "deploy", Title: "Deploy Commands:"},
		&cobra.Group{ID: "config", Title: "Configuration:"},
	)

	rootCmd.PersistentFlags().StringVarP(&environment, "environment", "e", "", "target environment (dev, stage, prod)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose/debug output")

	rootCmd.RegisterFlagCompletionFunc("environment", completeTargetNames)
}
