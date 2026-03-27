package cmd

import (
	"fmt"

	"github.com/example/kestrel/internal/helm"
	"github.com/spf13/cobra"
)

var deploymentCmd = &cobra.Command{
	Use:     "deployment",
	Short:   "Deployment information (stretch feature)",
	GroupID: "deploy",
}

var deploymentInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show deployment status across all configured targets",
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, name := range cfg.TargetNames() {
			fmt.Printf("target: %s\n", name)
			resolved, err := cfg.ResolveTarget(name)
			if err != nil {
				fmt.Printf("  (could not resolve: %v)\n", err)
				fmt.Println()
				continue
			}
			if err := helm.List(cfg, resolved); err != nil {
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
