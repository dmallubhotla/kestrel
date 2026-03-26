package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/example/kestrel/internal/awsconfig"
	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/swoop"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var swoopAutoconfigureForce bool

var swoopAutoconfigureCmd = &cobra.Command{
	Use:   "autoconfigure",
	Short: "Auto-discover terraform roots and map environments to AWS profiles",
	Long: `Walks the project to discover terraform roots, detects the IaC layout
(service-embedded or centralized), and maps environments to AWS profiles.

For service repos (misc/iac/live/{env}/ pattern), sets terraform.iac_dir
and creates environment entries for each env directory found.

For centralized IaC repos, creates environment entries for each top-level
account profile directory.

Existing environments from global config are reused when they match.
Use --force to overwrite existing environment mappings.`,
	RunE: runSwoopAutoconfigure,
}

func init() {
	swoopAutoconfigureCmd.Flags().BoolVar(&swoopAutoconfigureForce, "force", false, "overwrite existing environment mappings in .kestconfig")
	swoopCmd.AddCommand(swoopAutoconfigureCmd)
}

func runSwoopAutoconfigure(cmd *cobra.Command, args []string) error {
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

	// For service layout, also inspect the roots for account IDs (useful for matching).
	profiles := swoop.InspectProfiles(roots)
	for _, p := range profiles {
		if len(p.AccountIDs) > 0 {
			fmt.Printf("  Account IDs in %s: %s\n", p.Name, strings.Join(p.AccountIDs, ", "))
		}
	}

	// Step 3: Read AWS profiles for matching.
	awsProfiles, awsErr := awsconfig.ReadProfileDetails()
	if awsErr != nil {
		fmt.Printf("\nNote: could not read AWS profiles: %v\n", awsErr)
		fmt.Println("Skipping AWS profile matching. You can add aws_profile manually.")
		awsProfiles = nil
	} else if len(awsProfiles) > 0 {
		names := make([]string, len(awsProfiles))
		for i, p := range awsProfiles {
			names[i] = p.Name
		}
		fmt.Printf("\nFound %d AWS profile(s): %s\n", len(awsProfiles), strings.Join(names, ", "))
	}

	// Step 4: Build environment map.
	// Start from global config environments as a base — service repos should
	// reuse the globally configured environments.
	envMap := make(map[string]config.EnvConfig)

	// Collect account IDs across all profile dirs for matching.
	allAccountIDs := make(map[string][]string) // env name → account IDs
	for _, p := range profiles {
		if len(p.AccountIDs) > 0 {
			allAccountIDs[p.Name] = p.AccountIDs
		}
	}

	for _, envName := range layout.EnvNames {
		// Check if this environment already exists in global config.
		if cfg != nil {
			if existing, ok := cfg.Environments[envName]; ok {
				envMap[envName] = existing
				fmt.Printf("\n  %s → reusing global config (aws: %s)\n", envName, existing.AwsProfile)
				continue
			}
		}

		// Try to match to an AWS profile.
		var awsProfileName string
		if len(awsProfiles) > 0 {
			// Build a synthetic ProfileInfo for matching.
			pi := swoop.ProfileInfo{
				Name:       envName,
				AccountIDs: allAccountIDs[envName],
			}
			// Also check parent profile's account IDs for service repos
			// where the env name (dev) may differ from the discovery profile (misc).
			if layout.Type == "service" {
				for _, p := range profiles {
					if len(p.AccountIDs) > 0 && len(pi.AccountIDs) == 0 {
						pi.AccountIDs = p.AccountIDs
					}
				}
			}

			bestIdx := bestAWSProfileMatch(pi, awsProfiles)
			if bestIdx >= 0 {
				awsProfileName = awsProfiles[bestIdx].Name
				fmt.Printf("\n  %s → %s (auto-matched)\n", envName, awsProfileName)
			} else {
				fmt.Println()
				selected, err := promptAWSProfile(pi, awsProfiles)
				if err != nil {
					return err
				}
				awsProfileName = selected
			}
		}

		envMap[envName] = config.EnvConfig{
			AwsProfile: awsProfileName,
		}
	}

	// Step 5: Build proposed config.
	proposed := &config.Config{
		Environments: envMap,
	}

	// Set iac_dir for service repos.
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
		proposed = mergeAutoconfigureResult(existing, proposed, swoopAutoconfigureForce)
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
	fmt.Printf("\nConfig written to %s\n", configPath)
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

// mergeAutoconfigureResult merges autoconfigure's proposed config into
// an existing config. Without force, existing environments are preserved.
func mergeAutoconfigureResult(existing, proposed *config.Config, force bool) *config.Config {
	result := *existing

	// Set iac_dir if proposed and not already set (or force).
	if proposed.Terraform.IACDir != "" && (result.Terraform.IACDir == "" || force) {
		result.Terraform.IACDir = proposed.Terraform.IACDir
	}

	if result.Environments == nil {
		result.Environments = make(map[string]config.EnvConfig)
	}

	for name, env := range proposed.Environments {
		if _, exists := result.Environments[name]; exists && !force {
			continue
		}
		// Preserve existing kube_context when only adding aws_profile.
		if ex, ok := result.Environments[name]; ok {
			if env.KubeContext == "" && ex.KubeContext != "" {
				env.KubeContext = ex.KubeContext
			}
		}
		result.Environments[name] = env
	}

	return &result
}

// bestAWSProfileMatch finds the best AWS profile match for a profile/env.
// Returns -1 if no confident match.
func bestAWSProfileMatch(profileDir swoop.ProfileInfo, awsProfiles []awsconfig.Profile) int {
	bestIdx := -1
	bestScore := 0

	for i, ap := range awsProfiles {
		score := 0
		apLower := strings.ToLower(ap.Name)
		pdLower := strings.ToLower(profileDir.Name)

		if apLower == pdLower {
			score += 20
		} else if strings.Contains(apLower, pdLower) || strings.Contains(pdLower, apLower) {
			score += 5
		}

		if len(profileDir.AccountIDs) > 0 {
			ssoAccountID := awsProfileField(ap, "sso_account_id")
			for _, id := range profileDir.AccountIDs {
				if ssoAccountID != "" && id == ssoAccountID {
					score += 15
					break
				}
			}
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestScore < 5 {
		return -1
	}
	return bestIdx
}

func awsProfileField(p awsconfig.Profile, key string) string {
	for _, f := range p.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

func promptAWSProfile(profileDir swoop.ProfileInfo, awsProfiles []awsconfig.Profile) (string, error) {
	m := singleSelectModel{
		title: fmt.Sprintf("AWS profile for [%s]", profileDir.Name),
	}
	m.items = make([]selectItem, 0, len(awsProfiles)+1)
	m.items = append(m.items, selectItem{name: "(none)"})
	for _, ap := range awsProfiles {
		m.items = append(m.items, selectItem{
			name:    ap.Name,
			preview: formatAWSProfilePreview(ap),
		})
	}

	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	sm := result.(singleSelectModel)
	if sm.cancelled || sm.cursor == 0 {
		return "", nil
	}
	return awsProfiles[sm.cursor-1].Name, nil
}

func formatAWSProfilePreview(p awsconfig.Profile) string {
	if len(p.Fields) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range p.Fields {
		fmt.Fprintf(&b, "  %s = %s\n", f.Key, f.Value)
	}
	return b.String()
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

	tmpDir, err := os.MkdirTemp("", "kest-swoop-autoconfigure-*")
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
