package swoop

import (
	"testing"

	"github.com/example/kestrel/internal/config"
)

func TestResolveAWSProfile_MatchesRootProfile(t *testing.T) {
	cfg := &config.Config{
		Environments: map[string]config.EnvConfig{
			"dev": {AwsProfile: "acme-dev"},
			"prd": {AwsProfile: "acme-prd"},
		},
	}
	root := Root{Profile: "dev"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "acme-dev" {
		t.Errorf("got %q, want %q", got, "acme-dev")
	}
}

func TestResolveAWSProfile_FallsBackToActiveEnv(t *testing.T) {
	cfg := &config.Config{
		Environments: map[string]config.EnvConfig{
			"staging": {AwsProfile: "acme-staging"},
		},
	}
	// Root profile "prd" doesn't match any environment.
	root := Root{Profile: "prd"}
	got := ResolveAWSProfile(root, cfg, "staging")
	if got != "acme-staging" {
		t.Errorf("got %q, want %q", got, "acme-staging")
	}
}

func TestResolveAWSProfile_NilConfig(t *testing.T) {
	root := Root{Profile: "dev"}
	got := ResolveAWSProfile(root, nil, "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveAWSProfile_NoMatch(t *testing.T) {
	cfg := &config.Config{
		Environments: map[string]config.EnvConfig{
			"dev": {AwsProfile: "acme-dev"},
		},
	}
	root := Root{Profile: "unknown"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveAWSProfile_AccountIDFallback(t *testing.T) {
	// ci and prd share account ID; only prd has a profile configured.
	cfg := &config.Config{
		Environments: map[string]config.EnvConfig{
			"prd": {AwsAccountID: "593671994769", AwsProfile: "prd-sso"},
			"ci":  {AwsAccountID: "593671994769"},
		},
	}
	root := Root{Profile: "ci"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "prd-sso" {
		t.Errorf("got %q, want %q", got, "prd-sso")
	}
}

func TestResolveAWSProfile_AccountIDFallbackMultipleDirs(t *testing.T) {
	// Three directories share the same account; only one has a profile.
	cfg := &config.Config{
		Environments: map[string]config.EnvConfig{
			"prd":        {AwsAccountID: "593671994769", AwsProfile: "prd-sso"},
			"ci":         {AwsAccountID: "593671994769"},
			"example-prod": {AwsAccountID: "593671994769"},
			"dev":        {AwsAccountID: "585912155334", AwsProfile: "dev-sso"},
		},
	}

	tests := []struct {
		profile string
		want    string
	}{
		{"ci", "prd-sso"},
		{"example-prod", "prd-sso"},
		{"prd", "prd-sso"},
		{"dev", "dev-sso"},
	}

	for _, tt := range tests {
		root := Root{Profile: tt.profile}
		got := ResolveAWSProfile(root, cfg, "")
		if got != tt.want {
			t.Errorf("profile %q: got %q, want %q", tt.profile, got, tt.want)
		}
	}
}

func TestResolveAWSProfile_DirectMatchTakesPrecedence(t *testing.T) {
	// ci has its own profile despite sharing an account with prd.
	cfg := &config.Config{
		Environments: map[string]config.EnvConfig{
			"prd": {AwsAccountID: "593671994769", AwsProfile: "prd-sso"},
			"ci":  {AwsAccountID: "593671994769", AwsProfile: "ci-sso"},
		},
	}
	root := Root{Profile: "ci"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "ci-sso" {
		t.Errorf("got %q, want %q", got, "ci-sso")
	}
}

func TestResolveAWSProfile_AccountIDNoProfileAnywhere(t *testing.T) {
	// Account ID set but no profile on any env with that account.
	cfg := &config.Config{
		Environments: map[string]config.EnvConfig{
			"azure": {AwsAccountID: ""},
			"prd":   {AwsAccountID: "593671994769"},
		},
	}
	root := Root{Profile: "prd"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveAWSProfile_AccountIDFallbackThenActiveEnv(t *testing.T) {
	// Root profile not in config at all — should fall through to active env.
	cfg := &config.Config{
		Environments: map[string]config.EnvConfig{
			"dev": {AwsAccountID: "585912155334", AwsProfile: "dev-sso"},
		},
	}
	root := Root{Profile: "unknown"}
	got := ResolveAWSProfile(root, cfg, "dev")
	if got != "dev-sso" {
		t.Errorf("got %q, want %q", got, "dev-sso")
	}
}
