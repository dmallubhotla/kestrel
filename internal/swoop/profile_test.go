package swoop

import (
	"testing"

	"github.com/deepak-science/kestrel/internal/config"
)

func TestResolveAWSProfile_DirectoryMapping(t *testing.T) {
	cfg := &config.Config{
		Directories: map[string]string{
			"prd": "444455556666",
			"dev": "111122223333",
		},
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"444455556666": {AwsProfile: "prd-sso"},
				"111122223333": {AwsProfile: "dev-sso"},
			},
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
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"111122223333": {AwsProfile: "dev-sso"},
			},
		},
	}

	root := Root{Dir: "dev", AccountID: "111122223333"}
	got := ResolveAWSProfile(root, cfg, "")
	if got != "dev-sso" {
		t.Errorf("got %q, want %q", got, "dev-sso")
	}
}

func TestResolveAWSProfile_DirectoryOverridesAutoDiscovery(t *testing.T) {
	cfg := &config.Config{
		Directories: map[string]string{
			"global": "444455556666",
		},
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"444455556666": {AwsProfile: "prd-sso"},
				"111122223333": {AwsProfile: "other"},
			},
		},
	}

	// Root auto-discovered a different account, but directory mapping wins.
	root := Root{Dir: "global", AccountID: "111122223333"}
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
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"111122223333": {AwsProfile: "dev-sso"},
			},
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
		Kubernetes: config.KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev": "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
			},
		},
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"111122223333": {AwsProfile: "dev-sso"},
			},
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
			"prd":          "444455556666",
			"ci":           "444455556666",
			"example-prod": "444455556666",
		},
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"444455556666": {AwsProfile: "prd-sso"},
			},
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
