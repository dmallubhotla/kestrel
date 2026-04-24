package helm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/runner"
)

func envMap(awsProfile string) map[string]string {
	if awsProfile == "" {
		return nil
	}
	return map[string]string{"AWS_PROFILE": awsProfile}
}

// Deploy runs helm upgrade --install for a single release with 3-layer
// values coalescence:
//
//  1. shared.yaml           — common to all releases (optional, included if exists)
//  2. <target>.yaml         — target-specific (optional, included if exists)
//  3. release.Values[...]   — release-specific (required, error if missing)
func Deploy(cfg *config.Config, release config.HelmRelease, resolved config.ResolvedTarget, tag string, extraArgs []string) error {
	valuesDir := cfg.Helm.ValuesDir

	args := []string{
		"upgrade",
		"--namespace", cfg.Helm.Namespace,
		"--atomic",
		"--cleanup-on-fail",
		"--install",
		"--history-max", "0",
		"--timeout", "5m0s",
		"--kube-context", resolved.KubeContext,
	}

	// Layer 1: shared.yaml (always, if exists)
	sharedValues := filepath.Join(valuesDir, "shared.yaml")
	if _, err := os.Stat(sharedValues); err == nil {
		args = append(args, "--values", sharedValues)
		fmt.Fprintf(os.Stderr, "info: including %s\n", sharedValues)
	}

	// Layer 2: <target>.yaml (auto from target name, if exists)
	targetValues := filepath.Join(valuesDir, release.Target+".yaml")
	if _, err := os.Stat(targetValues); err == nil {
		args = append(args, "--values", targetValues)
		fmt.Fprintf(os.Stderr, "info: including %s\n", targetValues)
	}

	// Layer 3: release-specific values files (required)
	for _, v := range release.Values {
		vPath := filepath.Join(valuesDir, v)
		if _, err := os.Stat(vPath); err != nil {
			return fmt.Errorf("values file not found: %s (release %q)", vPath, release.ReleaseName)
		}
		args = append(args, "--values", vPath)
		fmt.Fprintf(os.Stderr, "info: including %s\n", vPath)
	}

	// Set image tag
	args = append(args, "--set", "image.tag="+tag)

	// Release name and chart
	args = append(args, release.ReleaseName, cfg.Helm.Chart)

	// Extra args passed through
	args = append(args, extraArgs...)

	return runner.RunWithEnv(envMap(resolved.AwsProfile), "helm", args...)
}

// List shows deployment info for a release.
func List(cfg *config.Config, release config.HelmRelease, resolved config.ResolvedTarget) error {
	return runner.RunWithEnv(envMap(resolved.AwsProfile), "helm", "ls",
		"--kube-context", resolved.KubeContext,
		"-n", cfg.Helm.Namespace,
		"--filter", release.ReleaseName,
	)
}

// Uninstall removes a helm release.
func Uninstall(cfg *config.Config, release config.HelmRelease, resolved config.ResolvedTarget) error {
	return runner.RunWithEnv(envMap(resolved.AwsProfile), "helm", "uninstall",
		"--namespace", cfg.Helm.Namespace,
		"--wait",
		"--timeout", "5m0s",
		"--kube-context", resolved.KubeContext,
		release.ReleaseName,
	)
}

// ResolveTag figures out the image tag to deploy, matching the bash script logic.
func ResolveTag(targetName, tagOverride string) (string, error) {
	if tagOverride != "" {
		return tagOverride, nil
	}

	if targetName == "prod" {
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
