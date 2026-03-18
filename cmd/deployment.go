package cmd

import (
	"fmt"

	"github.com/example/kestrel/internal/helm"
	"github.com/spf13/cobra"
)

var deploymentCmd = &cobra.Command{
	Use:   "deployment",
	Short: "Deployment information (stretch feature)",
}

var deploymentInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show deployment status across all configured environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		for envName, envCfg := range cfg.Environments {
			fmt.Printf("env: %s\n", envName)
			if err := helm.List(cfg, envCfg.KubeContext); err != nil {
				fmt.Printf("  (could not retrieve info: %v)\n", err)
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	deploymentCmd.AddCommand(deploymentInfoCmd)
	rootCmd.AddCommand(deploymentCmd)
}
