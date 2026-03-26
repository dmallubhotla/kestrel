package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/example/kestrel/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Show configuration details",
	GroupID: "config",
}

var configPathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "Show config file locations and which ones are loaded",
	RunE: func(cmd *cobra.Command, args []string) error {
		globalPath := config.GlobalConfigPath()

		fmt.Println("Config file locations:")
		fmt.Println()

		if cfg.Sources.Global != "" {
			fmt.Printf("  global:  %s (loaded)\n", cfg.Sources.Global)
		} else {
			fmt.Printf("  global:  %s (not found)\n", globalPath)
		}

		if cfg.Sources.Project != "" {
			fmt.Printf("  project: %s (loaded)\n", cfg.Sources.Project)
		} else {
			fmt.Println("  project: (none found)")
		}

		fmt.Println()
		fmt.Println("Merge order: global -> project (project overrides global)")

		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the resolved (merged) configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshalling config: %w", err)
		}
		fmt.Print(string(out))
		return nil
	},
}

var configEnvsCmd = &cobra.Command{
	Use:   "envs",
	Short: "List configured environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfg.Environments) == 0 {
			fmt.Println("No environments configured.")
			return nil
		}

		names := make([]string, 0, len(cfg.Environments))
		for name := range cfg.Environments {
			names = append(names, name)
		}
		sort.Strings(names)

		var rows []string
		for _, name := range names {
			env := cfg.Environments[name]
			parts := []string{name}
			if env.KubeContext != "" {
				parts = append(parts, fmt.Sprintf("context=%s", env.KubeContext))
			}
			if env.AwsProfile != "" {
				parts = append(parts, fmt.Sprintf("aws=%s", env.AwsProfile))
			}
			rows = append(rows, strings.Join(parts, "  "))
		}

		for _, row := range rows {
			fmt.Println(row)
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(configPathsCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEnvsCmd)
	rootCmd.AddCommand(configCmd)
}
