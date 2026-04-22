package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/swoop"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var swoopSetupForce bool

var swoopSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Discover terraform roots and initialize project .kestconfig",
	Long: `Walks the project to discover terraform roots, detects the IaC layout
(service-embedded or centralized), and writes infrastructure facts to .kestconfig.

For service repos (misc/iac/live/{env}/ pattern), sets terraform.iac_dir
and creates target entries for each env directory found.

For centralized IaC repos, creates directory→account mappings.

Use --force to overwrite existing mappings.`,
	RunE: runSwoopSetup,
}

func init() {
	swoopSetupCmd.Flags().BoolVar(&swoopSetupForce, "force", false, "overwrite existing mappings in .kestconfig")
	swoopCmd.AddCommand(swoopSetupCmd)
}

func runSwoopSetup(cmd *cobra.Command, args []string) error {
	// Step 1: Discover roots from project root (not iac_dir).
	projectRoot, err := resolveProjectRoot()
	if err != nil {
		return err
	}

	roots, err := swoop.Discover(projectRoot)
	if err != nil {
		return fmt.Errorf("discovering roots: %w", err)
	}
	if len(roots) == 0 {
		return fmt.Errorf("no terraform roots found under %s", projectRoot)
	}

	// Step 2: Detect layout.
	layout := swoop.DetectLayout(roots)

	fmt.Printf("Found %d terraform root(s)\n", len(roots))
	if layout.Type == "service" {
		fmt.Printf("Detected: service-embedded IaC (iac_dir: %s)\n", layout.IACDir)
	} else {
		fmt.Println("Detected: centralized IaC layout")
	}
	fmt.Printf("Directories: %s\n", strings.Join(layout.EnvNames, ", "))

	// Step 3: Inspect roots for account IDs.
	dirInfos := swoop.InspectDirs(roots, projectRoot)
	swoop.EnrichWithAccountIDs(roots, projectRoot)

	for _, p := range dirInfos {
		if len(p.AccountIDs) > 0 {
			fmt.Printf("  Account IDs in %s: %s\n", p.Name, strings.Join(p.AccountIDs, ", "))
		}
	}

	// Build per-root account ID map for service repos.
	// For service layout, the env dir is the part after "live/" in the root path.
	rootAccountIDs := make(map[string]string) // env name → account ID
	for _, r := range roots {
		if r.AccountID == "" {
			continue
		}
		// For service repos, extract env name from path (e.g., "misc/iac/live/dev" → "dev").
		envName := layout.EnvNameFromRoot(r)
		if envName != "" {
			// Keep first discovered — multiple roots in same env should agree.
			if _, exists := rootAccountIDs[envName]; !exists {
				rootAccountIDs[envName] = r.AccountID
			}
		}
	}

	// Collect account IDs across all top-level dirs (for centralized repos).
	allAccountIDs := make(map[string]string) // dir name → first account ID
	for _, p := range dirInfos {
		if len(p.AccountIDs) > 0 {
			allAccountIDs[p.Name] = p.AccountIDs[0]
		}
	}

	// Step 4: Build proposed config.
	proposed := &config.Config{}

	if layout.Type == "service" {
		proposed.Terraform = config.TerraformConfig{IACDir: layout.IACDir}

		// Service repos: create targets from env dirs under live/.
		proposed.Targets = make(map[string]config.TargetConfig)
		for _, envName := range layout.EnvNames {
			proposed.Targets[envName] = config.TargetConfig{}
		}

		// Also create directories map for per-target account IDs.
		if len(rootAccountIDs) > 0 {
			proposed.Directories = make(map[string]string)
			for envName, accountID := range rootAccountIDs {
				proposed.Directories[envName] = accountID
			}
		}
	} else {
		// Centralized repos: create directory→account mappings.
		proposed.Directories = make(map[string]string)
		for dirName, accountID := range allAccountIDs {
			proposed.Directories[dirName] = accountID
		}
	}

	// Determine project config path and load existing.
	configPath, existing, err := resolveProjectConfigPath(projectRoot)
	if err != nil {
		return err
	}

	// Merge with existing project config if present.
	if existing != nil {
		proposed = mergeSetupResult(existing, proposed, swoopSetupForce)
	}

	// Step 5: Preview and confirm.
	for {
		fmt.Println("\nProposed .kestconfig:")
		fmt.Println(strings.Repeat("─", 40))
		out, _ := yaml.Marshal(proposed)
		fmt.Print(string(out))
		fmt.Println(strings.Repeat("─", 40))
		fmt.Printf("Path: %s\n", configPath)

		cm := confirmModel{}
		result, err := runTUI(cm)
		if err != nil {
			return err
		}
		choice := result.(confirmModel).choice

		switch choice {
		case choiceCancel:
			fmt.Println("Cancelled.")
			return nil
		case choiceEdit:
			edited, err := editConfigInEditor(proposed)
			if err != nil {
				return err
			}
			if edited == nil {
				fmt.Println("Editor returned empty file, keeping previous version.")
				continue
			}
			proposed = edited
			continue
		case choiceWrite:
			// fall through
		}
		break
	}

	if err := config.WriteToPath(proposed, configPath); err != nil {
		return err
	}
	fmt.Printf("\nProject config written to %s\n", configPath)

	fmt.Println("\nTo configure access credentials (aws_profile, kube_context), run:")
	fmt.Println("  kest config autoconfigure")
	return nil
}

// resolveProjectRoot returns the project root directory.
func resolveProjectRoot() (string, error) {
	if cfg != nil && cfg.Sources.Project != "" {
		return filepath.Abs(filepath.Dir(cfg.Sources.Project))
	}
	return os.Getwd()
}

// resolveProjectConfigPath determines where to write the .kestconfig and
// loads any existing config from that path.
func resolveProjectConfigPath(projectRoot string) (string, *config.Config, error) {
	if cfg != nil && cfg.Sources.Project != "" {
		existing, err := config.LoadFromPath(cfg.Sources.Project)
		if err != nil {
			return "", nil, fmt.Errorf("reading existing .kestconfig: %w", err)
		}
		return cfg.Sources.Project, existing, nil
	}

	path := filepath.Join(projectRoot, ".kestconfig")
	return path, nil, nil
}

func editConfigInEditor(c *config.Config) (*config.Config, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshalling config: %w", err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	tmpDir, err := os.MkdirTemp("", "kest-swoop-setup-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, ".kestconfig")
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return nil, fmt.Errorf("writing temp file: %w", err)
	}

	fmt.Println("\nOpening config in editor for review...")
	editorCmd := exec.Command(editor, tmpFile)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return nil, fmt.Errorf("editor exited with error: %w", err)
	}

	edited, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("reading edited file: %w", err)
	}
	if len(strings.TrimSpace(string(edited))) == 0 {
		return nil, nil
	}

	var result config.Config
	if err := yaml.Unmarshal(edited, &result); err != nil {
		return nil, fmt.Errorf("parsing edited config: %w", err)
	}
	return &result, nil
}

// mergeSetupResult merges setup's proposed config into an existing config.
func mergeSetupResult(existing, proposed *config.Config, force bool) *config.Config {
	result := *existing

	// Set iac_dir if proposed and not already set (or force).
	if proposed.Terraform.IACDir != "" && (result.Terraform.IACDir == "" || force) {
		result.Terraform.IACDir = proposed.Terraform.IACDir
	}

	// Merge targets.
	if result.Targets == nil {
		result.Targets = make(map[string]config.TargetConfig)
	}
	for name, tc := range proposed.Targets {
		if _, exists := result.Targets[name]; !exists || force {
			result.Targets[name] = tc
		}
	}

	// Merge directories.
	if result.Directories == nil && len(proposed.Directories) > 0 {
		result.Directories = make(map[string]string)
	}
	for name, accountID := range proposed.Directories {
		if _, exists := result.Directories[name]; !exists || force {
			result.Directories[name] = accountID
		}
	}

	return &result
}
