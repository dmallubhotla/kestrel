package swoop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePlanSummary_Changes(t *testing.T) {
	output := `
Terraform will perform the following actions:

  # aws_instance.example will be created

Plan: 1 to add, 2 to change, 0 to destroy.

Changes to Outputs:
`
	got := parsePlanSummary(output)
	want := "1 to add, 2 to change, 0 to destroy"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParsePlanSummary_NoChanges(t *testing.T) {
	output := `
No changes. Your infrastructure matches the configuration.

Terraform has compared your real infrastructure against your configuration
`
	got := parsePlanSummary(output)
	if got != "no changes" {
		t.Errorf("got %q, want %q", got, "no changes")
	}
}

func TestParsePlanSummary_Empty(t *testing.T) {
	got := parsePlanSummary("some random output\nwith no plan line\n")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestParseTerraformVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Terraform v1.9.2\non linux_amd64\n", "1.9.2"},
		{"Terraform v1.0.0\n", "1.0.0"},
		{"some garbage\n", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseTerraformVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseTerraformVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEnsureTFVersion_SkipsWhenAlreadySet(t *testing.T) {
	root := Root{
		AbsPath:   t.TempDir(),
		TFVersion: "1.9.2",
	}
	got, err := EnsureTFVersion(root, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty version, got %q", got)
	}
	// File should not have been created.
	if _, err := os.Stat(filepath.Join(root.AbsPath, ".terraform-version")); err == nil {
		t.Error("expected no .terraform-version file, but one exists")
	}
}

func TestEnsureTFVersion_WritesPreferredVersion(t *testing.T) {
	dir := t.TempDir()
	root := Root{AbsPath: dir}

	got, err := EnsureTFVersion(root, "1.5.7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.5.7" {
		t.Errorf("expected 1.5.7, got %q", got)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".terraform-version"))
	if err != nil {
		t.Fatalf("could not read .terraform-version: %v", err)
	}
	if string(data) != "1.5.7\n" {
		t.Errorf("file content = %q, want %q", string(data), "1.5.7\n")
	}
}
