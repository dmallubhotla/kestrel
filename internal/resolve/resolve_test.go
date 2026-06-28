package resolve

import (
	"testing"

	"github.com/deepak-science/kestrel/internal/config"
)

func TestAWSProfileForRoot_DirectoryMapping(t *testing.T) {
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

	got := AWSProfileForRoot(cfg, "prd", "", "")
	if got != "prd-sso" {
		t.Errorf("got %q, want %q", got, "prd-sso")
	}
}

func TestAWSProfileForRoot_AutoDiscoveredAccount(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"111122223333": {AwsProfile: "dev-sso"},
			},
		},
	}

	got := AWSProfileForRoot(cfg, "dev", "111122223333", "")
	if got != "dev-sso" {
		t.Errorf("got %q, want %q", got, "dev-sso")
	}
}

func TestAWSProfileForRoot_DirectoryOverridesAutoDiscovery(t *testing.T) {
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

	got := AWSProfileForRoot(cfg, "global", "111122223333", "")
	if got != "prd-sso" {
		t.Errorf("got %q, want %q", got, "prd-sso")
	}
}

func TestAWSProfileForRoot_ActiveTargetFallback(t *testing.T) {
	cfg := &config.Config{
		Targets: map[string]config.TargetConfig{
			"dev": {Cluster: "eks-dev", AWSAccount: "111122223333"},
		},
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"111122223333": {AwsProfile: "dev-sso"},
			},
		},
		Kubernetes: config.KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev": "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
			},
		},
	}

	got := AWSProfileForRoot(cfg, "unknown", "", "dev")
	if got != "dev-sso" {
		t.Errorf("got %q, want %q", got, "dev-sso")
	}
}

func TestAWSProfileForRoot_NilConfig(t *testing.T) {
	got := AWSProfileForRoot(nil, "dev", "", "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestAWSProfileForRoot_NoMatch(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"111122223333": {AwsProfile: "dev-sso"},
			},
		},
	}

	got := AWSProfileForRoot(cfg, "unknown", "", "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestAccountIDForRoot_DirectoryMapping(t *testing.T) {
	cfg := &config.Config{
		Directories: map[string]string{
			"prd": "444455556666",
		},
	}

	got := AccountIDForRoot(cfg, "prd", "", "")
	if got != "444455556666" {
		t.Errorf("got %q, want 444455556666", got)
	}
}

func TestAccountIDForRoot_AutoDiscovered(t *testing.T) {
	cfg := &config.Config{}

	got := AccountIDForRoot(cfg, "dev", "111122223333", "")
	if got != "111122223333" {
		t.Errorf("got %q, want 111122223333", got)
	}
}

func TestAccountIDForRoot_ActiveTargetFallback(t *testing.T) {
	cfg := &config.Config{
		Targets: map[string]config.TargetConfig{
			"dev": {AWSAccount: "111122223333"},
		},
	}

	got := AccountIDForRoot(cfg, "unknown", "", "dev")
	if got != "111122223333" {
		t.Errorf("got %q, want 111122223333", got)
	}
}
