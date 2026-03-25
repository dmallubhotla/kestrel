package awsconfig

import (
	"strings"
	"testing"
)

func TestParseProfiles(t *testing.T) {
	content := `[default]
region = us-east-1

[profile acme-dev]
region = us-east-1

[profile acme-staging]
region = us-west-2

[profile acme-prod]
region = us-east-1

[profile personal]
region = eu-west-1
`

	profiles, err := ParseProfiles(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"acme-dev", "acme-prod", "acme-staging", "default", "personal"}
	if len(profiles) != len(expected) {
		t.Fatalf("got %d profiles, want %d: %v", len(profiles), len(expected), profiles)
	}
	for i, name := range expected {
		if profiles[i] != name {
			t.Errorf("profile[%d] = %q, want %q", i, profiles[i], name)
		}
	}
}

func TestParseProfilesEmpty(t *testing.T) {
	profiles, err := ParseProfiles(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Fatalf("expected no profiles, got %v", profiles)
	}
}

func TestInferEnv(t *testing.T) {
	tests := []struct {
		profile string
		want    string
	}{
		{"acme-dev", "dev"},
		{"acme-staging", "stage"},
		{"acme-prod", "prod"},
		{"acme-production", "prod"},
		{"developer-tools", "dev"},
		{"personal", ""},
		{"default", ""},
	}

	for _, tt := range tests {
		got := InferEnv(tt.profile)
		if got != tt.want {
			t.Errorf("InferEnv(%q) = %q, want %q", tt.profile, got, tt.want)
		}
	}
}
