package resolve

import (
	"testing"

	"github.com/example/kestrel/internal/config"
)

func TestAWSProfileForRoot_DirectoryMapping(t *testing.T) {
	cfg := &config.Config{
		Directories: map[string]string{
			"prd": "593671994769",
			"dev": "585912155334",
		},
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"593671994769": {AwsProfile: "prd-sso"},
				"585912155334": {AwsProfile: "dev-sso"},
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
				"585912155334": {AwsProfile: "dev-sso"},
			},
		},
	}

	got := AWSProfileForRoot(cfg, "dev", "585912155334", "")
	if got != "dev-sso" {
		t.Errorf("got %q, want %q", got, "dev-sso")
	}
}

func TestAWSProfileForRoot_DirectoryOverridesAutoDiscovery(t *testing.T) {
	cfg := &config.Config{
		Directories: map[string]string{
			"global": "593671994769",
		},
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"593671994769": {AwsProfile: "prd-sso"},
				"111111111111": {AwsProfile: "other"},
			},
		},
	}

	got := AWSProfileForRoot(cfg, "global", "111111111111", "")
	if got != "prd-sso" {
		t.Errorf("got %q, want %q", got, "prd-sso")
	}
}

func TestAWSProfileForRoot_ActiveTargetFallback(t *testing.T) {
	cfg := &config.Config{
		Targets: map[string]config.TargetConfig{
			"dev": {Cluster: "eks-dev", AWSAccount: "585912155334"},
		},
		AWS: config.AWSConfig{
			Accounts: map[string]config.AWSAccountConfig{
				"585912155334": {AwsProfile: "dev-sso"},
			},
		},
		Kubernetes: config.KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev": "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
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
				"585912155334": {AwsProfile: "dev-sso"},
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
			"prd": "593671994769",
		},
	}

	got := AccountIDForRoot(cfg, "prd", "", "")
	if got != "593671994769" {
		t.Errorf("got %q, want 593671994769", got)
	}
}

func TestAccountIDForRoot_AutoDiscovered(t *testing.T) {
	cfg := &config.Config{}

	got := AccountIDForRoot(cfg, "dev", "585912155334", "")
	if got != "585912155334" {
		t.Errorf("got %q, want 585912155334", got)
	}
}

func TestAccountIDForRoot_ActiveTargetFallback(t *testing.T) {
	cfg := &config.Config{
		Targets: map[string]config.TargetConfig{
			"dev": {AWSAccount: "585912155334"},
		},
	}

	got := AccountIDForRoot(cfg, "unknown", "", "dev")
	if got != "585912155334" {
		t.Errorf("got %q, want 585912155334", got)
	}
}
