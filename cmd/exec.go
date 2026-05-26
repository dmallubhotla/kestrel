package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"github.com/dmallubhotla/kestrel/internal/runner"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:     "exec <command> [args...]",
	Short:   "Run a command with the active target's environment",
	GroupID: "deploy",
	Long: `Executes the given command after configuring the shell environment
for the active target:

  • Sets AWS_PROFILE if the target's cluster resolves to an AWS account
  • Switches kubectl context if the target defines a cluster

Use -- to separate kest flags from the wrapped command's flags:

  kest -e dev exec -- kubectl get pods
  kest exec -- stern -n my-ns my-service
  kest exec -- helm list -A`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: false,
	SilenceUsage:       true,
	SilenceErrors:      true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if environment == "" {
			return fmt.Errorf("no target: use -e <target> or set a profile with 'kest profile use'")
		}

		resolved, err := cfg.ResolveTarget(environment)
		if err != nil {
			return err
		}

		if err := ensureSSOSession(resolved.AwsProfile); err != nil {
			return err
		}

		// Switch kube context before exec (modifies kubeconfig state).
		if resolved.KubeContext != "" {
			slog.Info("switching kube context", "context", resolved.KubeContext)
			if verbose {
				fmt.Fprintf(os.Stderr, "debug: switching kube context to %s\n", resolved.KubeContext)
			}
			if err := runner.Run("kubectl", "config", "use-context", resolved.KubeContext); err != nil {
				return fmt.Errorf("failed to switch kube context: %w", err)
			}
		}

		// Build environment with AWS_PROFILE injected.
		environ := os.Environ()
		if resolved.AwsProfile != "" {
			environ = append(environ, "AWS_PROFILE="+resolved.AwsProfile)
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
