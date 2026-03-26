package swoop

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// ExecResult captures the outcome of a terraform execution.
type ExecResult struct {
	// ExitCode is the process exit code.
	ExitCode int

	// PlanSummary is the parsed plan summary line, if any (e.g. "1 to add, 0 to change, 0 to destroy").
	// Empty for non-plan commands or if parsing fails.
	PlanSummary string
}

// TFVersionCheck is the result of checking the terraform version for a root.
type TFVersionCheck struct {
	// OK is true if the version matches or no version constraint exists.
	OK bool

	// Required is the version from .terraform-version (empty if none).
	Required string

	// Installed is the version terraform reports from the root dir.
	Installed string

	// TfenvAvailable is true if tfenv is on PATH.
	TfenvAvailable bool
}

// RunTerraform executes terraform with the given args in the root's directory.
// Output is streamed to stdout/stderr. The awsProfile is injected as AWS_PROFILE
// if non-empty.
func RunTerraform(root Root, awsProfile string, args ...string) (*ExecResult, error) {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = root.AbsPath
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if awsProfile != "" {
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "AWS_PROFILE="+awsProfile)
	}

	// For plan, capture stdout to parse the summary while still streaming it.
	var stdoutBuf bytes.Buffer
	isPlan := len(args) > 0 && args[0] == "plan"
	if isPlan {
		cmd.Stdout = &teeWriter{w: os.Stdout, buf: &stdoutBuf}
	} else {
		cmd.Stdout = os.Stdout
	}

	err := cmd.Run()
	result := &ExecResult{}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if isPlan {
		result.PlanSummary = parsePlanSummary(stdoutBuf.String())
	}

	return result, err
}

// teeWriter writes to both w and buf.
type teeWriter struct {
	w   *os.File
	buf *bytes.Buffer
}

func (t *teeWriter) Write(p []byte) (int, error) {
	t.buf.Write(p)
	return t.w.Write(p)
}

// parsePlanSummary extracts the plan summary from terraform plan output.
var planSummaryRe = regexp.MustCompile(`Plan: (.+?)\.$`)

func parsePlanSummary(output string) string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "No changes") {
			return "no changes"
		}
		if m := planSummaryRe.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// CheckTFVersion checks whether the terraform version available in the root
// directory matches the .terraform-version requirement.
func CheckTFVersion(root Root) TFVersionCheck {
	if root.TFVersion == "" {
		return TFVersionCheck{OK: true}
	}

	// Run terraform version from the root dir so tfenv picks up .terraform-version.
	cmd := exec.Command("terraform", "version")
	cmd.Dir = root.AbsPath
	out, err := cmd.Output()
	if err != nil {
		tfenv := hasTfenv()
		return TFVersionCheck{
			Required:       root.TFVersion,
			TfenvAvailable: tfenv,
		}
	}

	installed := parseTerraformVersion(string(out))
	if installed == "" {
		return TFVersionCheck{OK: true, Required: root.TFVersion}
	}

	tfenv := hasTfenv()
	return TFVersionCheck{
		OK:             installed == root.TFVersion,
		Required:       root.TFVersion,
		Installed:      installed,
		TfenvAvailable: tfenv,
	}
}

// InstallTFVersion runs `tfenv install <version>` with output streamed to stderr.
func InstallTFVersion(version string) error {
	cmd := exec.Command("tfenv", "install", version)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func hasTfenv() bool {
	_, err := exec.LookPath("tfenv")
	return err == nil
}

var tfVersionRe = regexp.MustCompile(`Terraform v(\d+\.\d+\.\d+)`)

func parseTerraformVersion(output string) string {
	m := tfVersionRe.FindStringSubmatch(output)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// FormatTFVersionCommand returns the command string that would install the version.
func FormatTFVersionCommand(version string) string {
	return fmt.Sprintf("tfenv install %s", version)
}
