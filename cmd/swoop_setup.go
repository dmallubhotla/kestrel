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
and creates environment entries for each env directory found.

For centralized IaC repos, creates environment entries for each top-level
account profile directory.

Infrastructure fields (aws_account_id) are written to the project .kestconfig.
Use --force to overwrite existing environment mappings.`,
	RunE: runSwoopSetup,
}

func init() {
	swoopSetupCmd.Flags().BoolVar(&swoopSetupForce, "force", false, "overwrite existing environment mappings in .kestconfig")
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
	fmt.Printf("Environments: %s\n", strings.Join(layout.EnvNames, ", "))

	// Step 3: Inspect roots for account IDs.
	profiles := swoop.InspectProfiles(roots, projectRoot)
	for _, p := range profiles {
		if len(p.AccountIDs) > 0 {
			fmt.Printf("  Account IDs in %s: %s\n", p.Name, strings.Join(p.AccountIDs, ", "))
		}
	}

	// Collect account IDs across all profile dirs.
	allAccountIDs := make(map[string][]string) // env name → account IDs
	for _, p := range profiles {
		if len(p.AccountIDs) > 0 {
			allAccountIDs[p.Name] = p.AccountIDs
		}
	}

	// Step 4: Build environment map with infrastructure fields only.
	projectEnvMap := make(map[string]config.EnvConfig)

	for _, envName := range layout.EnvNames {
		var projectEnv config.EnvConfig
		accountIDs := allAccountIDs[envName]
		// For service repos, fall back to parent profile's account IDs.
		if layout.Type == "service" && len(accountIDs) == 0 {
			for _, p := range profiles {
				if len(p.AccountIDs) > 0 {
					accountIDs = p.AccountIDs
					break
				}
			}
		}
		if len(accountIDs) > 0 {
			projectEnv.AwsAccountID = accountIDs[0]
		}
		projectEnvMap[envName] = projectEnv
	}

	// Step 5: Build proposed project config (infra fields only).
	proposed := &config.Config{
		Environments: projectEnvMap,
	}
	if layout.Type == "service" {
		proposed.Terraform = config.TerraformConfig{
			IACDir: layout.IACDir,
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

	// Step 6: Preview and confirm.
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
// Uses the .kestconfig location if found, otherwise cwd.
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

// mergeSetupResult merges init's proposed config into an existing config.
// Without force, existing environments are preserved.
// Uses field-level merge so existing fields are not clobbered.
func mergeSetupResult(existing, proposed *config.Config, force bool) *config.Config {
	result := *existing

	// Set iac_dir if proposed and not already set (or force).
	if proposed.Terraform.IACDir != "" && (result.Terraform.IACDir == "" || force) {
		result.Terraform.IACDir = proposed.Terraform.IACDir
	}

	if result.Environments == nil {
		result.Environments = make(map[string]config.EnvConfig)
	}

	for name, env := range proposed.Environments {
		if ex, exists := result.Environments[name]; exists && !force {
			// Field-level merge: overlay proposed onto existing.
			result.Environments[name] = config.MergeEnvField(ex, env)
		} else {
			result.Environments[name] = env
		}
	}

	return &result
}
