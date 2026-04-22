package swoop

import (
	"testing"

	"github.com/example/kestrel/internal/config"
)

func TestResolveAWSProfile_DirectoryMapping(t *testing.T) {
	cfg := &config.Config{
		Directories: map[string]string{
			"prd": "593671994769",
			"dev": "585912155334",
		},
		Accounts: map[string]config.AccountConfig{
			"593671994769": {AwsProfile: "prd-sso"},
			"585912155334": {AwsProfile: "dev-sso"},
		},
	}

	root := Root{Dir: "prd"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "prd-sso" {
		t.Errorf("got %q, want %q", got, "prd-sso")
	}
}

func TestResolveAWSProfile_AccountIDOnRoot(t *testing.T) {
	cfg := &config.Config{
		Accounts: map[string]config.AccountConfig{
			"585912155334": {AwsProfile: "dev-sso"},
		},
	}

	root := Root{Dir: "dev", AccountID: "585912155334"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "dev-sso" {
		t.Errorf("got %q, want %q", got, "dev-sso")
	}
}

func TestResolveAWSProfile_DirectoryOverridesAutoDiscovery(t *testing.T) {
	cfg := &config.Config{
		Directories: map[string]string{
			"global": "593671994769",
		},
		Accounts: map[string]config.AccountConfig{
			"593671994769": {AwsProfile: "prd-sso"},
			"111111111111": {AwsProfile: "other"},
		},
	}

	// Root auto-discovered a different account, but directory mapping wins.
	root := Root{Dir: "global", AccountID: "111111111111"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "prd-sso" {
		t.Errorf("got %q, want %q", got, "prd-sso")
	}
}

func TestResolveAWSProfile_NilConfig(t *testing.T) {
	root := Root{Dir: "dev"}
	got := ResolveAWSProfile(root, nil, "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveAWSProfile_NoMatch(t *testing.T) {
	cfg := &config.Config{
		Accounts: map[string]config.AccountConfig{
			"585912155334": {AwsProfile: "dev-sso"},
		},
	}
	root := Root{Dir: "unknown"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveAWSProfile_FallsBackToActiveTarget(t *testing.T) {
	cfg := &config.Config{
		Targets: map[string]config.TargetConfig{
			"dev": {Cluster: "eks-dev"},
		},
		Contexts: map[string]string{
			"eks-dev": "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
		},
		Accounts: map[string]config.AccountConfig{
			"585912155334": {AwsProfile: "dev-sso"},
		},
	}

	root := Root{Dir: "unknown"}
	got := ResolveAWSProfile(root, cfg, "dev")
	if got != "dev-sso" {
		t.Errorf("got %q, want %q", got, "dev-sso")
	}
}

func TestResolveAWSProfile_MultipleDirsSameAccount(t *testing.T) {
	cfg := &config.Config{
		Directories: map[string]string{
			"prd":        "593671994769",
			"ci":         "593671994769",
			"example-prod": "593671994769",
		},
		Accounts: map[string]config.AccountConfig{
			"593671994769": {AwsProfile: "prd-sso"},
		},
	}

	for _, profile := range []string{"prd", "ci", "example-prod"} {
		root := Root{Dir: profile}
		got := ResolveAWSProfile(root, cfg, "")
		if got != "prd-sso" {
			t.Errorf("profile %q: got %q, want %q", profile, got, "prd-sso")
		}
	}
}
