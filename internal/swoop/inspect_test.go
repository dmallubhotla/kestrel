package swoop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectDirs_BasicCounts(t *testing.T) {
	base := t.TempDir()

	createTFRoot(t, filepath.Join(base, "dev", "vpc"))
	createTFRoot(t, filepath.Join(base, "dev", "rds"))
	createTFRoot(t, filepath.Join(base, "prd", "vpc"))

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	dirs := InspectDirs(roots, base)
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}

	// Sorted alphabetically.
	if dirs[0].Name != "dev" || dirs[0].RootCount != 2 {
		t.Errorf("dev: got name=%q count=%d", dirs[0].Name, dirs[0].RootCount)
	}
	if dirs[1].Name != "prd" || dirs[1].RootCount != 1 {
		t.Errorf("prd: got name=%q count=%d", dirs[1].Name, dirs[1].RootCount)
	}
}

func TestInspectDirs_AccountIDExtraction(t *testing.T) {
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

	dirs := InspectDirs(roots, base)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}

	if len(dirs[0].AccountIDs) != 1 || dirs[0].AccountIDs[0] != "585912155334" {
		t.Errorf("expected account ID 585912155334, got %v", dirs[0].AccountIDs)
	}
}

func TestInspectDirs_HCLAccountID(t *testing.T) {
	base := t.TempDir()

	// Create a terragrunt.hcl at the account level with aws_account_id.
	accountDir := filepath.Join(base, "prd")
	os.MkdirAll(accountDir, 0o755)
	os.WriteFile(filepath.Join(accountDir, "terragrunt.hcl"), []byte(`
remote_state {
  backend = "s3"
  config = {
    bucket = "my-bucket"
  }
}

inputs = {
  aws_account_id = "593671994769"
}
`), 0o644)

	// Create a terraform root nested under the account dir.
	root := filepath.Join(accountDir, "us-east-1", "prod", "vpc")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "main.tf"), []byte(`
terraform {
  backend "s3" {
    bucket = "my-bucket"
    key    = "prod/vpc"
  }
}
`), 0o644)

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}

	dirs := InspectDirs(roots, base)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}

	if len(dirs[0].AccountIDs) != 1 || dirs[0].AccountIDs[0] != "593671994769" {
		t.Errorf("expected account ID 593671994769 from ancestor HCL, got %v", dirs[0].AccountIDs)
	}
}

func TestInspectDirs_HCLInRoot(t *testing.T) {
	base := t.TempDir()

	// Root dir has both a .tf backend and a .hcl with account_id.
	root := filepath.Join(base, "dev", "vpc")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "main.tf"), []byte(`
terraform {
  backend "s3" {
    bucket = "my-bucket"
  }
}
`), 0o644)
	os.WriteFile(filepath.Join(root, "terragrunt.hcl"), []byte(`
inputs = {
  account_id = "585912155334"
}
`), 0o644)

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	dirs := InspectDirs(roots, base)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}
	if len(dirs[0].AccountIDs) != 1 || dirs[0].AccountIDs[0] != "585912155334" {
		t.Errorf("expected account ID 585912155334 from HCL in root, got %v", dirs[0].AccountIDs)
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
