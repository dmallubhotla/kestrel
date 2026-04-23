package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/example/kestrel/internal/execlog"
)

// Run executes a command, streaming stdout/stderr to the terminal.
func Run(name string, args ...string) error {
	return RunInDir("", name, args...)
}

// RunInDir executes a command in a specific directory.
func RunInDir(dir, name string, args ...string) error {
	return RunInDirWithEnv(dir, nil, name, args...)
}

// RunWithEnv executes a command with additional environment variables.
func RunWithEnv(env map[string]string, name string, args ...string) error {
	return RunInDirWithEnv("", env, name, args...)
}

// RunInDirWithEnv executes a command in a specific directory with additional env vars.
func RunInDirWithEnv(dir string, env map[string]string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if len(env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	fmt.Fprintf(os.Stderr, "debug: %s %s\n", name, strings.Join(args, " "))

	start := time.Now()
	err := cmd.Run()
	logCommand(name, args, dir, start, err)
	return err
}

// Output executes a command and returns its stdout.
func Output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	start := time.Now()
	out, err := cmd.Output()
	logCommand(name, args, "", start, err)
	return strings.TrimSpace(string(out)), err
}

func logCommand(name string, args []string, dir string, start time.Time, err error) {
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	execlog.Log(execlog.Entry{
		Timestamp:  start,
		Command:    name,
		Args:       args,
		Dir:        dir,
		ExitCode:   exitCode,
		DurationMs: time.Since(start).Milliseconds(),
	})
}
