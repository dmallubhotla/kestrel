package swoop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepak-science/kestrel/internal/config"
)

// writeRoot creates a temp dir containing a single main.tf and returns a Root
// pointing at it.
func writeRoot(t *testing.T, dir, tf string) Root {
	t.Helper()
	abs := t.TempDir()
	if err := os.WriteFile(filepath.Join(abs, "main.tf"), []byte(tf), 0o644); err != nil {
		t.Fatalf("write main.tf: %v", err)
	}
	return Root{Path: dir, AbsPath: abs, Dir: dir}
}

func cfgWithAccount(accountID, profile string) *config.Config {
	cfg := &config.Config{}
	cfg.AWS.Accounts = map[string]config.AWSAccountConfig{
		accountID: {AwsProfile: profile},
	}
	return cfg
}

const backendAssumeRoleTF = `terraform {
  backend "s3" {
    bucket      = "state"
    key         = "k"
    region      = "us-east-2"
    assume_role = { role_arn = "arn:aws:iam::111122223333:role/tfstate_backend_role" }
  }
}

provider "cloudflare" {}
`

// A repo whose only AWS touchpoint is an S3 backend assume_role: no
// provider/account markers. The effective profile must fall
// back to the backend account's profile — this is exactly the resolution that
// used to be missing from the kestci path.
func TestEffectiveProfiles_BackendFallback(t *testing.T) {
	cfg := cfgWithAccount("111122223333", "homelab")
	root := writeRoot(t, "cloudflare", backendAssumeRoleTF)

	res := EffectiveProfiles(cfg, root, "")

	if res.ProviderProfile != "" {
		t.Errorf("ProviderProfile = %q, want empty (no account/target mapping)", res.ProviderProfile)
	}
	if res.BackendProfile != "homelab" {
		t.Errorf("BackendProfile = %q, want homelab", res.BackendProfile)
	}
	if res.Effective != "homelab" {
		t.Errorf("Effective = %q, want homelab (backend fallback)", res.Effective)
	}
	if res.AccountID != "111122223333" {
		t.Errorf("AccountID = %q, want 111122223333", res.AccountID)
	}
}

// When a root maps to a profile via an explicit account marker, that provider
// profile wins and a same-account backend is not surfaced as a separate cred.
func TestEffectiveProfiles_ProviderProfileWins(t *testing.T) {
	cfg := cfgWithAccount("111122223333", "homelab")
	tf := `terraform {
  backend "s3" {
    assume_role = { role_arn = "arn:aws:iam::111122223333:role/tfstate_backend_role" }
  }
}

provider "aws" {
  allowed_account_ids = ["111122223333"]
}
`
	root := writeRoot(t, "infra", tf)
	// Simulate discovery enrichment having found the account marker.
	root.AccountID = "111122223333"

	res := EffectiveProfiles(cfg, root, "")

	if res.ProviderProfile != "homelab" {
		t.Errorf("ProviderProfile = %q, want homelab", res.ProviderProfile)
	}
	if res.BackendProfile != "" {
		t.Errorf("BackendProfile = %q, want empty (same account as provider)", res.BackendProfile)
	}
	if res.Effective != "homelab" {
		t.Errorf("Effective = %q, want homelab", res.Effective)
	}
}

// No config and no backend auth must not panic and must resolve to nothing.
func TestEffectiveProfiles_NilConfig(t *testing.T) {
	root := writeRoot(t, "plain", "terraform {\n  backend \"local\" {}\n}\n")

	res := EffectiveProfiles(nil, root, "")

	if res.ProviderProfile != "" || res.BackendProfile != "" || res.Effective != "" {
		t.Errorf("expected all-empty resolution, got %+v", res)
	}
}
