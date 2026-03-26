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
