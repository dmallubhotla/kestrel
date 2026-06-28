package cmd

import (
	"github.com/deepak-science/kestrel/internal/swoop"
	"github.com/spf13/cobra"
)

// completeTargetNames returns configured target names for shell completion.
func completeTargetNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return cfg.TargetNames(), cobra.ShellCompDirectiveNoFileComp
}

// completeDeployNames returns configured deploy keys for shell completion.
func completeDeployNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return cfg.DeployNames(), cobra.ShellCompDirectiveNoFileComp
}

// completeSwoopRoots returns discovered terraform root paths for shell completion.
func completeSwoopRoots(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	baseDir, err := resolveBaseDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	roots, err := swoop.Discover(baseDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	paths := make([]string, len(roots))
	for i, r := range roots {
		paths[i] = r.Path
	}
	return paths, cobra.ShellCompDirectiveNoFileComp
}

// completeSwoopDirs returns unique top-level directory names for shell completion.
func completeSwoopDirs(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	roots, err := swoop.Discover(baseDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]bool{}
	var dirs []string
	for _, r := range roots {
		if r.Dir != "" && !seen[r.Dir] {
			seen[r.Dir] = true
			dirs = append(dirs, r.Dir)
		}
	}
	return dirs, cobra.ShellCompDirectiveNoFileComp
}
