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
// Looks for lines like "Plan: 1 to add, 2 to change, 0 to destroy." or
// "No changes. Your infrastructure matches the configuration."
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

// CheckTFVersion validates that the required terraform version (from .terraform-version)
// is available. Returns a user-friendly warning message if not, or empty string if OK.
func CheckTFVersion(root Root) string {
	if root.TFVersion == "" {
		return ""
	}

	// Check if terraform is available at all.
	out, err := exec.Command("terraform", "version").Output()
	if err != nil {
		return fmt.Sprintf("warning: terraform not found on PATH")
	}

	// Parse the installed version from "Terraform vX.Y.Z" line.
	installed := parseTerraformVersion(string(out))
	if installed == "" {
		return ""
	}

	if installed != root.TFVersion {
		return fmt.Sprintf("warning: root requires terraform %s (from .terraform-version) but %s is active\n  If using tfenv: tfenv install %s && tfenv use %s",
			root.TFVersion, installed, root.TFVersion, root.TFVersion)
	}
	return ""
}

var tfVersionRe = regexp.MustCompile(`Terraform v(\d+\.\d+\.\d+)`)

func parseTerraformVersion(output string) string {
	m := tfVersionRe.FindStringSubmatch(output)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
