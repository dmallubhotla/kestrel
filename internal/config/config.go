package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Helm         HelmConfig              `yaml:"helm"`
	Terraform    TerraformConfig         `yaml:"terraform"`
	Environments map[string]EnvConfig    `yaml:"environments"`
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

const configFileName = ".kestconfig"

// Load reads the global ~/.kestconfig and the project-level .kestconfig,
// merging them with project-level values taking precedence.
func Load() (*Config, error) {
	global, err := loadGlobal()
	if err != nil {
		return nil, fmt.Errorf("loading global config: %w", err)
	}

	project, err := loadProject()
	if err != nil {
		return nil, fmt.Errorf("loading project config: %w", err)
	}

	merged := merge(global, project)

	// Apply defaults
	if merged.Helm.Namespace == "" {
		merged.Helm.Namespace = "app"
	}

	return merged, nil
}

func loadGlobal() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return &Config{}, nil
	}
	return loadFile(filepath.Join(home, configFileName))
}

func loadProject() (*Config, error) {
	path, err := findProjectConfig()
	if err != nil {
		return &Config{}, nil
	}
	return loadFile(path)
}

// findProjectConfig walks up from the current directory looking for .kestconfig.
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

// ResolveEnv returns the environment config for the given name, or an error.
func (c *Config) ResolveEnv(name string) (EnvConfig, error) {
	env, ok := c.Environments[name]
	if !ok {
		return EnvConfig{}, fmt.Errorf("environment %q not configured (check your .kestconfig)", name)
	}
	return env, nil
}
