package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/example/kestrel/internal/runner"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <command> [args...]",
	Short: "Run a command with the active profile's environment",
	Long: `Executes the given command after configuring the shell environment
for the active profile:

  • Sets AWS_PROFILE if the profile defines aws_profile
  • Switches kubectl context if the profile defines kube_context

Use -- to separate kest flags from the wrapped command's flags:

  kest -e dev exec -- kubectl get pods
  kest exec -- stern -n my-ns my-service
  kest exec -- helm list -A`,
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagParsing:    false,
	SilenceUsage:          true,
	SilenceErrors:         true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("no environment: use -e <env> or set a profile with 'kest profile use'")
		}

		envCfg, err := cfg.ResolveEnv(environment)
		if err != nil {
			return err
		}

		// Switch kube context before exec (modifies kubeconfig state).
		if envCfg.KubeContext != "" {
			if verbose {
				fmt.Fprintf(os.Stderr, "debug: switching kube context to %s\n", envCfg.KubeContext)
			}
			if err := runner.Run("kubectl", "config", "use-context", envCfg.KubeContext); err != nil {
				return fmt.Errorf("failed to switch kube context: %w", err)
			}
		}

		// Build environment with AWS_PROFILE injected.
		environ := os.Environ()
		if envCfg.AwsProfile != "" {
			environ = append(environ, "AWS_PROFILE="+envCfg.AwsProfile)
		}

		// Resolve the binary path and exec (replaces this process).
		binary, err := exec.LookPath(args[0])
		if err != nil {
			return fmt.Errorf("command not found: %s", args[0])
		}

		return syscall.Exec(binary, args, environ)
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
