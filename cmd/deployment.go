package cmd

import (
	"fmt"

	"github.com/dmallubhotla/kestrel/internal/helm"
	"github.com/spf13/cobra"
)

var deploymentCmd = &cobra.Command{
	Use:     "deployment",
	Short:   "Deployment information (stretch feature)",
	GroupID: "deploy",
}

var deploymentInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show deployment status across all configured releases",
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, name := range cfg.ReleaseNames() {
			release := cfg.Helm.Releases[name]
			fmt.Printf("release: %s (%s) → target %s\n", name, release.ReleaseName, release.Target)
			resolved, err := cfg.ResolveTarget(release.Target)
			if err != nil {
				fmt.Printf("  (could not resolve: %v)\n", err)
				fmt.Println()
				continue
			}
			if err := helm.List(cfg, release, resolved); err != nil {
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
