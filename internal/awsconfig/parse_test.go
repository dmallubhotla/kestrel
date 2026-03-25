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

func TestParseProfileDetails(t *testing.T) {
	content := `[default]
region = us-east-1
output = json

[profile acme-dev]
region = us-east-1
sso_start_url = https://acme.awsapps.com/start
sso_account_id = 111111111111
sso_role_name = DevAccess
sso_region = us-east-1

[profile acme-prod]
region = us-east-1
role_arn = arn:aws:iam::123456789012:role/ProdRole
source_profile = default
`

	profiles, err := ParseProfileDetails(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	if len(profiles) != 3 {
		t.Fatalf("got %d profiles, want 3: %v", len(profiles), profiles)
	}

	// Sorted: acme-dev, acme-prod, default
	if profiles[0].Name != "acme-dev" {
		t.Errorf("profiles[0].Name = %q, want acme-dev", profiles[0].Name)
	}
	if len(profiles[0].Fields) != 5 {
		t.Errorf("acme-dev has %d fields, want 5", len(profiles[0].Fields))
	}

	if profiles[2].Name != "default" {
		t.Errorf("profiles[2].Name = %q, want default", profiles[2].Name)
	}
	if len(profiles[2].Fields) != 2 {
		t.Errorf("default has %d fields, want 2", len(profiles[2].Fields))
	}
}

func TestParseProfileDetailsSensitiveFiltered(t *testing.T) {
	content := `[profile creds]
region = us-east-1
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
aws_session_token = FwoGZXIvYXdzEBY
output = json
`

	profiles, err := ParseProfileDetails(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	if len(profiles) != 1 {
		t.Fatalf("got %d profiles, want 1", len(profiles))
	}

	// Only region and output should remain (3 sensitive keys filtered).
	if len(profiles[0].Fields) != 2 {
		t.Errorf("got %d fields, want 2 (sensitive keys should be filtered)", len(profiles[0].Fields))
		for _, f := range profiles[0].Fields {
			t.Logf("  %s = %s", f.Key, f.Value)
		}
	}

	for _, f := range profiles[0].Fields {
		if sensitiveKeys[f.Key] {
			t.Errorf("sensitive key %q was not filtered", f.Key)
		}
	}
}
