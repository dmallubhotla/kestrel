package swoop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectProfiles_BasicCounts(t *testing.T) {
	base := t.TempDir()

	createTFRoot(t, filepath.Join(base, "dev", "vpc"))
	createTFRoot(t, filepath.Join(base, "dev", "rds"))
	createTFRoot(t, filepath.Join(base, "prd", "vpc"))

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	profiles := InspectProfiles(roots)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}

	// Sorted alphabetically.
	if profiles[0].Name != "dev" || profiles[0].RootCount != 2 {
		t.Errorf("dev: got name=%q count=%d", profiles[0].Name, profiles[0].RootCount)
	}
	if profiles[1].Name != "prd" || profiles[1].RootCount != 1 {
		t.Errorf("prd: got name=%q count=%d", profiles[1].Name, profiles[1].RootCount)
	}
}

func TestInspectProfiles_AccountIDExtraction(t *testing.T) {
	base := t.TempDir()

	root := filepath.Join(base, "dev", "vpc")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "root.tf"), []byte(`
terraform {
  backend "s3" {
    bucket = "my-bucket"
    key    = "dev/vpc/terraform.tfstate"
  }
}

provider "aws" {
  region              = "us-east-1"
  allowed_account_ids = ["585912155334"]
}
`), 0o644)

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	profiles := InspectProfiles(roots)
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	if len(profiles[0].AccountIDs) != 1 || profiles[0].AccountIDs[0] != "585912155334" {
		t.Errorf("expected account ID 585912155334, got %v", profiles[0].AccountIDs)
	}
}

func TestExtractAccountIDs_NoMatch(t *testing.T) {
	base := t.TempDir()
	os.WriteFile(filepath.Join(base, "main.tf"), []byte(`
provider "aws" {
  region = "us-east-1"
}
`), 0o644)

	ids := extractAccountIDs(base)
	if len(ids) != 0 {
		t.Errorf("expected no IDs, got %v", ids)
	}
}
