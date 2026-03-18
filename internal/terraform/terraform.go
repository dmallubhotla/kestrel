package terraform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/runner"
)

// Run proxies a terraform command in the appropriate env directory.
func Run(cfg *config.Config, env string, tfArgs []string) error {
	dir := filepath.Join(cfg.Terraform.IACDir, "live", env)

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("terraform directory not found: %s", dir)
	}

	fmt.Fprintf(os.Stderr, "debug: using %s...\n", dir)
	return runner.RunInDir(dir, "terraform", tfArgs...)
}
