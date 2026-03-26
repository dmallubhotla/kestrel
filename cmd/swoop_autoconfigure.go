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
	Short: "Auto-discover terraform roots and map profile directories to AWS profiles",
	Long: `Walks the project to discover terraform roots, identifies account profile
directories, and attempts to match them to AWS profiles from ~/.aws/config.

When a match is ambiguous, you'll be prompted to select the correct AWS profile.
The result is written to the project's .kestconfig file.

Existing config is preserved unless --force is used.`,
	RunE: runSwoopAutoconfigure,
}

func init() {
	swoopAutoconfigureCmd.Flags().BoolVar(&swoopAutoconfigureForce, "force", false, "overwrite existing environment mappings in .kestconfig")
	swoopCmd.AddCommand(swoopAutoconfigureCmd)
}

func runSwoopAutoconfigure(cmd *cobra.Command, args []string) error {
	// Step 1: Discover roots.
	baseDir, err := resolveBaseDirForAutoconfigure()
	if err != nil {
		return err
	}

	roots, err := swoop.Discover(baseDir)
	if err != nil {
		return fmt.Errorf("discovering roots: %w", err)
	}
	if len(roots) == 0 {
		return fmt.Errorf("no terraform roots found under %s", baseDir)
	}

	// Step 2: Analyze profile directories.
	profiles := swoop.InspectProfiles(roots)

	fmt.Printf("Found %d terraform root(s) across %d profile(s):\n", len(roots), len(profiles))
	for _, p := range profiles {
		acctInfo := ""
		if len(p.AccountIDs) > 0 {
			acctInfo = fmt.Sprintf("  (account: %s)", strings.Join(p.AccountIDs, ", "))
		}
		fmt.Printf("  %s: %d root(s)%s\n", p.Name, p.RootCount, acctInfo)
	}

	// Detect archetype.
	if len(profiles) == 1 && profiles[0].Name == "live" {
		fmt.Println("\nDetected: service-embedded IaC layout (live/{env}/ pattern)")
	} else {
		fmt.Println("\nDetected: centralized IaC layout (multi-account profile directories)")
	}

	// Step 3: Read AWS profiles.
	awsProfiles, awsErr := awsconfig.ReadProfileDetails()
	if awsErr != nil {
		fmt.Printf("\nNote: could not read AWS profiles: %v\n", awsErr)
		fmt.Println("Skipping AWS profile matching. You can add aws_profile manually to .kestconfig.")
		awsProfiles = nil
	} else if len(awsProfiles) > 0 {
		names := make([]string, len(awsProfiles))
		for i, p := range awsProfiles {
			names[i] = p.Name
		}
		fmt.Printf("\nFound %d AWS profile(s): %s\n", len(awsProfiles), strings.Join(names, ", "))
	}

	// Step 4: Match profile dirs to AWS profiles.
	envMap := make(map[string]config.EnvConfig)

	for _, profileDir := range profiles {
		var awsProfileName string

		if len(awsProfiles) > 0 {
			bestIdx := bestAWSProfileMatch(profileDir, awsProfiles)

			if bestIdx >= 0 {
				// Confident match — show it and use it.
				awsProfileName = awsProfiles[bestIdx].Name
				fmt.Printf("\n  %s → %s (auto-matched)\n", profileDir.Name, awsProfileName)
			} else {
				// No confident match — prompt.
				fmt.Println()
				selected, err := promptAWSProfile(profileDir, awsProfiles)
				if err != nil {
					return err
				}
				awsProfileName = selected
			}
		}

		envMap[profileDir.Name] = config.EnvConfig{
			AwsProfile: awsProfileName,
		}
	}

	// Step 5: Build proposed config.
	proposed := &config.Config{
		Environments: envMap,
	}

	// Determine project config path.
	configPath, existing, err := resolveProjectConfigPath(baseDir)
	if err != nil {
		return err
	}

	// Merge with existing config if present.
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

// resolveBaseDirForAutoconfigure determines the directory to scan.
// Uses cwd by default since autoconfigure is typically run at the repo root.
func resolveBaseDirForAutoconfigure() (string, error) {
	// If terraform.iac_dir is configured, use it.
	if cfg != nil && cfg.Terraform.IACDir != "" {
		if cfg.Sources.Project != "" {
			projectRoot := filepath.Dir(cfg.Sources.Project)
			return filepath.Abs(filepath.Join(projectRoot, cfg.Terraform.IACDir))
		}
		return filepath.Abs(cfg.Terraform.IACDir)
	}

	return os.Getwd()
}

// resolveProjectConfigPath determines where to write the .kestconfig and
// loads any existing config from that path.
func resolveProjectConfigPath(baseDir string) (string, *config.Config, error) {
	// Check if there's already a .kestconfig.
	if cfg != nil && cfg.Sources.Project != "" {
		existing, err := config.LoadFromPath(cfg.Sources.Project)
		if err != nil {
			return "", nil, fmt.Errorf("reading existing .kestconfig: %w", err)
		}
		return cfg.Sources.Project, existing, nil
	}

	// No existing config — write to baseDir/.kestconfig.
	path := filepath.Join(baseDir, ".kestconfig")
	return path, nil, nil
}

// mergeAutoconfigureResult merges autoconfigure's proposed environments into
// an existing config. Without force, existing environments are preserved.
func mergeAutoconfigureResult(existing, proposed *config.Config, force bool) *config.Config {
	result := *existing

	if result.Environments == nil {
		result.Environments = make(map[string]config.EnvConfig)
	}

	for name, env := range proposed.Environments {
		if _, exists := result.Environments[name]; exists && !force {
			// Preserve existing environment config.
			continue
		}
		// Merge: keep existing kube_context if we're only adding aws_profile.
		if existing, ok := result.Environments[name]; ok {
			if env.KubeContext == "" && existing.KubeContext != "" {
				env.KubeContext = existing.KubeContext
			}
		}
		result.Environments[name] = env
	}

	return &result
}

// bestAWSProfileMatch finds the best AWS profile match for a terraform profile dir.
// Returns -1 if no confident match.
func bestAWSProfileMatch(profileDir swoop.ProfileInfo, awsProfiles []awsconfig.Profile) int {
	bestIdx := -1
	bestScore := 0

	for i, ap := range awsProfiles {
		score := 0
		apLower := strings.ToLower(ap.Name)
		pdLower := strings.ToLower(profileDir.Name)

		// Exact name match is strongest.
		if apLower == pdLower {
			score += 20
		} else if strings.Contains(apLower, pdLower) || strings.Contains(pdLower, apLower) {
			score += 5
		}

		// Account ID match from provider blocks vs sso_account_id.
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

	// Only return a match if we're reasonably confident.
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

// promptAWSProfile shows a single-select picker for choosing an AWS profile
// for a terraform profile directory.
func promptAWSProfile(profileDir swoop.ProfileInfo, awsProfiles []awsconfig.Profile) (string, error) {
	m := singleSelectModel{
		title: fmt.Sprintf("AWS profile for [%s] (%d roots)", profileDir.Name, profileDir.RootCount),
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

// editConfigInEditor opens the config in $EDITOR for manual editing.
func editConfigInEditor(cfg *config.Config) (*config.Config, error) {
	data, err := yaml.Marshal(cfg)
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
