package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Helm         HelmConfig           `yaml:"helm"`
	Terraform    TerraformConfig      `yaml:"terraform"`
	Environments map[string]EnvConfig `yaml:"environments"`

	// Sources tracks which config files were loaded (not serialised).
	Sources Sources `yaml:"-"`
}

// Sources records the file paths that contributed to the loaded config.
type Sources struct {
	Global  string // XDG config path (may be empty if not found)
	Project string // project-level .kestconfig (may be empty if not found)
}

type HelmConfig struct {
	Chart         string   `yaml:"chart"`
	ValuesDir     string   `yaml:"values_dir"`
	DeployScripts []string `yaml:"deploy_scripts"`
	ReleaseName   string   `yaml:"release_name"`
	Namespace     string   `yaml:"namespace"`
}

type TerraformConfig struct {
	IACDir string `yaml:"iac_dir"`
}

type EnvConfig struct {
	KubeContext string `yaml:"kube_context"`
	AwsProfile  string `yaml:"aws_profile"`
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

// Load reads the global XDG config and the project-level .kestconfig,
// merging them with project-level values taking precedence.
func Load() (*Config, error) {
	globalPath := GlobalConfigPath()
	global, err := loadFile(globalPath)
	if err != nil {
		return nil, fmt.Errorf("loading global config: %w", err)
	}

	projectPath, _ := findProjectConfig()
	project, err := loadFile(projectPath)
	if err != nil {
		return nil, fmt.Errorf("loading project config: %w", err)
	}

	merged := merge(global, project)

	// Record which files actually existed.
	if fileExists(globalPath) {
		merged.Sources.Global = globalPath
	}
	if projectPath != "" && fileExists(projectPath) {
		merged.Sources.Project = projectPath
	}

	// Apply defaults
	if merged.Helm.Namespace == "" {
		merged.Helm.Namespace = "app"
	}

	return merged, nil
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

// merge combines global and project configs. Project values override global.
func merge(global, project *Config) *Config {
	out := *global

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

	// Environments: project-level entries override global per-key
	if out.Environments == nil {
		out.Environments = make(map[string]EnvConfig)
	}
	for k, v := range project.Environments {
		out.Environments[k] = v
	}

	return &out
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
func (c *Config) ResolveEnv(name string) (EnvConfig, error) {
	env, ok := c.Environments[name]
	if !ok {
		return EnvConfig{}, fmt.Errorf("environment %q not configured (check your .kestconfig)", name)
	}
	return env, nil
}
