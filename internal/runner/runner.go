package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run executes a command, streaming stdout/stderr to the terminal.
func Run(name string, args ...string) error {
	return RunInDir("", name, args...)
}

// RunInDir executes a command in a specific directory.
func RunInDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Fprintf(os.Stderr, "debug: %s %s\n", name, strings.Join(args, " "))
	return cmd.Run()
}

// Output executes a command and returns its stdout.
func Output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
