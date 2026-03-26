package swoop

import "testing"

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
