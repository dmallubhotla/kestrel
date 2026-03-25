package helm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/runner"
)

// Deploy runs a helm upgrade --install with the appropriate flags,
// mirroring the logic from common-helm-deploy.sh.
func envMap(awsProfile string) map[string]string {
	if awsProfile == "" {
		return nil
	}
	return map[string]string{"AWS_PROFILE": awsProfile}
}

// Deploy runs a helm upgrade --install with the appropriate flags,
// mirroring the logic from common-helm-deploy.sh.
func Deploy(cfg *config.Config, env string, envCfg config.EnvConfig, tag string, extraArgs []string) error {
	valuesDir := cfg.Helm.ValuesDir

	args := []string{
		"upgrade",
		"--namespace", cfg.Helm.Namespace,
		"--atomic",
		"--cleanup-on-fail",
		"--install",
		"--history-max", "0",
		"--timeout", "5m0s",
		"--kube-context", envCfg.KubeContext,
	}

	// Layer values files: shared.yaml first, then <env>.yaml
	sharedValues := filepath.Join(valuesDir, "shared.yaml")
	if _, err := os.Stat(sharedValues); err == nil {
		args = append(args, "--values", sharedValues)
		fmt.Fprintf(os.Stderr, "info: including %s\n", sharedValues)
	}

	envValues := filepath.Join(valuesDir, env+".yaml")
	if _, err := os.Stat(envValues); err != nil {
		return fmt.Errorf("values file not found: %s (environment %q not supported?)", envValues, env)
	}
	args = append(args, "--values", envValues)

	// Set image tag
	args = append(args, "--set", "image.tag="+tag)

	// Release name and chart
	args = append(args, cfg.Helm.ReleaseName, cfg.Helm.Chart)

	// Extra args passed through
	args = append(args, extraArgs...)

	return runner.RunWithEnv(envMap(envCfg.AwsProfile), "helm", args...)
}

// List shows deployment info for a release.
func List(cfg *config.Config, envCfg config.EnvConfig) error {
	return runner.RunWithEnv(envMap(envCfg.AwsProfile), "helm", "ls",
		"--kube-context", envCfg.KubeContext,
		"-n", cfg.Helm.Namespace,
	)
}

// Uninstall removes a helm release.
func Uninstall(cfg *config.Config, envCfg config.EnvConfig) error {
	return runner.RunWithEnv(envMap(envCfg.AwsProfile), "helm", "uninstall",
		"--namespace", cfg.Helm.Namespace,
		"--wait",
		"--timeout", "5m0s",
		"--kube-context", envCfg.KubeContext,
		cfg.Helm.ReleaseName,
	)
}

// ResolveTag figures out the image tag to deploy, matching the bash script logic.
func ResolveTag(env, tagOverride string) (string, error) {
	if tagOverride != "" {
		return tagOverride, nil
	}

	if env == "prod" {
		// Use latest git tag
		tag, err := runner.Output("git", "describe", "--tags", "--abbrev=0")
		if err != nil {
			return "", fmt.Errorf("could not determine latest git tag for prod deploy: %w", err)
		}
		return tag, nil
	}

	// branch-shortsha
	branch, err := runner.Output("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not determine git branch: %w", err)
	}
	sha, err := runner.Output("git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not determine git sha: %w", err)
	}
	return branch + "-" + sha, nil
}
