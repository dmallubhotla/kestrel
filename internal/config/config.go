package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Helm         HelmConfig           `yaml:"helm,omitempty"`
	Terraform    TerraformConfig      `yaml:"terraform,omitempty"`
	Environments map[string]EnvConfig `yaml:"environments,omitempty"`

	// Raw layers preserved for commands that need source awareness.
	// ProjectEnvs holds environments from the project .kestconfig (infrastructure facts).
	// UserEnvs holds environments from the user's global config.yaml (access methods).
	// Not serialized — populated by Load().
	ProjectEnvs map[string]EnvConfig `yaml:"-"`
	UserEnvs    map[string]EnvConfig `yaml:"-"`

	// Sources tracks which config files were loaded (not serialised).
	Sources Sources `yaml:"-"`
}

// Sources records the file paths that contributed to the loaded config.
type Sources struct {
	Global  string // XDG config path (may be empty if not found)
	Project string // project-level .kestconfig (may be empty if not found)
}

type HelmConfig struct {
	Chart         string   `yaml:"chart,omitempty"`
	ValuesDir     string   `yaml:"values_dir,omitempty"`
	DeployScripts []string `yaml:"deploy_scripts,omitempty"`
	ReleaseName   string   `yaml:"release_name,omitempty"`
	Namespace     string   `yaml:"namespace,omitempty"`
}

type TerraformConfig struct {
	IACDir string `yaml:"iac_dir,omitempty"`
}

type EnvConfig struct {
	// Infrastructure facts (typically from project .kestconfig, safe to commit).
	AwsAccountID string `yaml:"aws_account_id,omitempty"`
	Region       string `yaml:"region,omitempty"`
	Cluster      string `yaml:"cluster,omitempty"`

	// Access methods (typically from user config.yaml, not committed).
	KubeContext string `yaml:"kube_context,omitempty"`
	AwsProfile  string `yaml:"aws_profile,omitempty"`
}

const (
	appName        = "kest"
	configFileName = ".kestconfig"
	globalFileName = "config.yaml"
)

// GlobalConfigPath returns the expected path for the global config file:
// $XDG_CONFIG_HOME/kest/config.yaml (typically ~/.config/kest/config.yaml).
func GlobalConfigPath() string {
	return filepath.Join(xdg.ConfigHome, appName, globalFileName)
}

// Load reads the global XDG config (user access methods) and the project-level
// .kestconfig (infrastructure facts), composing them into a resolved config.
//
// When a project .kestconfig defines environments, those are authoritative —
// user-only environments are not included. When no project config exists,
// user environments are used directly (backwards-compatible).
func Load() (*Config, error) {
	globalPath := GlobalConfigPath()
	user, err := loadFile(globalPath)
	if err != nil {
		return nil, fmt.Errorf("loading global config: %w", err)
	}

	projectPath, _ := findProjectConfig()
	project, err := loadFile(projectPath)
	if err != nil {
		return nil, fmt.Errorf("loading project config: %w", err)
	}

	out := compose(user, project)

	// Record which files actually existed.
	if fileExists(globalPath) {
		out.Sources.Global = globalPath
	}
	if projectPath != "" && fileExists(projectPath) {
		out.Sources.Project = projectPath
	}

	// Apply defaults
	if out.Helm.Namespace == "" {
		out.Helm.Namespace = "app"
	}

	return out, nil
}

func findProjectConfig() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, configFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found", configFileName)
		}
		dir = parent
	}
}

func loadFile(path string) (*Config, error) {
	if path == "" {
		return &Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// compose combines user (global) and project configs into a resolved Config.
//
// For Helm/Terraform settings, project values override user (same as before).
// For environments, project config is authoritative when it defines any:
// project provides infrastructure facts, user provides access methods,
// and the resolved env has all fields. When no project environments exist,
// user environments are used directly (backwards-compatible).
func compose(user, project *Config) *Config {
	out := *user

	// Helm: project overrides non-empty fields
	if project.Helm.Chart != "" {
		out.Helm.Chart = project.Helm.Chart
	}
	if project.Helm.ValuesDir != "" {
		out.Helm.ValuesDir = project.Helm.ValuesDir
	}
	if len(project.Helm.DeployScripts) > 0 {
		out.Helm.DeployScripts = project.Helm.DeployScripts
	}
	if project.Helm.ReleaseName != "" {
		out.Helm.ReleaseName = project.Helm.ReleaseName
	}
	if project.Helm.Namespace != "" {
		out.Helm.Namespace = project.Helm.Namespace
	}

	// Terraform
	if project.Terraform.IACDir != "" {
		out.Terraform.IACDir = project.Terraform.IACDir
	}

	// Preserve raw layers for source-aware commands.
	out.ProjectEnvs = project.Environments
	out.UserEnvs = user.Environments

	// Compose environments.
	out.Environments = composeEnvs(user.Environments, project.Environments)

	return &out
}

// composeEnvs builds the resolved environment map from user and project layers.
func composeEnvs(userEnvs, projectEnvs map[string]EnvConfig) map[string]EnvConfig {
	envs := make(map[string]EnvConfig)

	if len(projectEnvs) > 0 {
		// Project is authoritative: only project-defined environments are valid.
		for name, projEnv := range projectEnvs {
			env := projEnv
			// Layer in user access config.
			if userEnv, ok := userEnvs[name]; ok {
				if userEnv.AwsProfile != "" {
					env.AwsProfile = userEnv.AwsProfile
				}
				if userEnv.KubeContext != "" {
					env.KubeContext = userEnv.KubeContext
				}
			}
			envs[name] = env
		}
	} else {
		// No project environments — use user config directly (backwards-compatible).
		for name, userEnv := range userEnvs {
			envs[name] = userEnv
		}
	}

	return envs
}

const maxBackups = 3

// rotateBackups shifts existing .bak files up and copies the current file
// to .bak. Files beyond maxBackups are removed.
func rotateBackups(path string) {
	if _, err := os.Stat(path); err != nil {
		return // nothing to back up
	}

	// Remove oldest if it exists.
	oldest := fmt.Sprintf("%s.bak.%d", path, maxBackups)
	os.Remove(oldest)

	// Shift .bak.N → .bak.N+1
	for i := maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.bak.%d", path, i)
		dst := fmt.Sprintf("%s.bak.%d", path, i+1)
		os.Rename(src, dst)
	}

	// .bak → .bak.1
	os.Rename(path+".bak", path+".bak.1")

	// current → .bak
	data, err := os.ReadFile(path)
	if err == nil {
		os.WriteFile(path+".bak", data, 0o644)
	}
}

// WriteGlobal writes the given config to the global config path,
// creating parent directories as needed. The previous file is rotated
// into .bak, .bak.1, .bak.2, etc.
func WriteGlobal(cfg *Config) error {
	path := GlobalConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	rotateBackups(path)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// WriteToPath writes the given config to an arbitrary path with backup rotation.
func WriteToPath(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	rotateBackups(path)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// LoadFromPath reads a config from a specific file path.
func LoadFromPath(path string) (*Config, error) {
	return loadFile(path)
}

// LoadGlobal reads only the global config file (no project merge).
func LoadGlobal() (*Config, error) {
	return loadFile(GlobalConfigPath())
}

// ResolveEnv returns the environment config for the given name, or an error.
// It validates that required access fields are configured when infrastructure
// fields indicate they should be (e.g. aws_account_id set but aws_profile missing).
func (c *Config) ResolveEnv(name string) (EnvConfig, error) {
	env, ok := c.Environments[name]
	if !ok {
		return EnvConfig{}, fmt.Errorf("environment %q not configured (check your .kestconfig)", name)
	}

	// Validate access config when infra fields are present.
	if env.AwsAccountID != "" && env.AwsProfile == "" {
		return EnvConfig{}, fmt.Errorf(
			"environment %q has aws_account_id (%s) but no aws_profile configured\n"+
				"  Run 'kest config autoconfigure' or add aws_profile for %q to %s",
			name, env.AwsAccountID, name, GlobalConfigPath())
	}

	return env, nil
}

// HasProjectEnvs returns true if the config was loaded with project-level
// environment definitions (i.e. a .kestconfig with environments).
func (c *Config) HasProjectEnvs() bool {
	return len(c.ProjectEnvs) > 0
}

// MergeEnvField merges a single EnvConfig field-by-field into an existing entry.
func MergeEnvField(base, overlay EnvConfig) EnvConfig {
	if overlay.AwsAccountID != "" {
		base.AwsAccountID = overlay.AwsAccountID
	}
	if overlay.Region != "" {
		base.Region = overlay.Region
	}
	if overlay.Cluster != "" {
		base.Cluster = overlay.Cluster
	}
	if overlay.KubeContext != "" {
		base.KubeContext = overlay.KubeContext
	}
	if overlay.AwsProfile != "" {
		base.AwsProfile = overlay.AwsProfile
	}
	return base
}
