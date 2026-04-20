package awslogin

import (
	"fmt"
	"os"
	"os/exec"
)

// EnsureSession checks whether the given AWS profile has a valid session
// (via sts get-caller-identity) and, if not, runs aws sso login.
// This is interactive and should only be called from developer workflows.
func EnsureSession(profile string) error {
	if profile == "" {
		return nil
	}

	if sessionValid(profile) {
		return nil
	}

	fmt.Fprintf(os.Stderr, "AWS session expired for profile %q, running sso login...\n", profile)

	cmd := exec.Command("aws", "sso", "login", "--profile", profile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws sso login failed for profile %q: %w", profile, err)
	}

	return nil
}

// sessionValid returns true if sts get-caller-identity succeeds for the profile.
func sessionValid(profile string) bool {
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--profile", profile)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}
