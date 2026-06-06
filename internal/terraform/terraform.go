package terraform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmallubhotla/kestrel/internal/config"
	"github.com/dmallubhotla/kestrel/internal/runner"
)

// Run proxies a terraform command in the appropriate env directory.
func Run(cfg *config.Config, targetName string, resolved config.ResolvedTarget, tfArgs []string) error {
	dir := filepath.Join(cfg.Terraform.IACDir, "live", targetName)

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("terraform directory not found: %s", dir)
	}

	var extraEnv map[string]string
	if resolved.AwsProfile != "" {
		extraEnv = map[string]string{"AWS_PROFILE": resolved.AwsProfile}
	}

	fmt.Fprintf(os.Stderr, "debug: using %s...\n", dir)
	return runner.RunInDirWithEnv(dir, extraEnv, cfg.TerraformCommand(), tfArgs...)
}
