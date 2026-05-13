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

func TestExtractBackendAuth_AssumeRoleArn(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.tf"), []byte(`
terraform {
  backend "s3" {
    bucket      = "example-iac-tfstate"
    key         = "dev/vpc/terraform.tfstate"
    assume_role = { role_arn = "arn:aws:iam::593671994769:role/tf-runner" }
  }
}

provider "aws" {
  region              = "us-east-1"
  allowed_account_ids = ["585912155334"]
  assume_role { role_arn = "arn:aws:iam::585912155334:role/dev-deployer" }
}
`), 0o644)

	got := ExtractBackendAuth(dir)
	want := BackendAuth{AccountID: "593671994769"}
	if got != want {
		t.Errorf("ExtractBackendAuth = %+v, want %+v", got, want)
	}
}

func TestExtractBackendAuth_TopLevelRoleArn(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.tf"), []byte(`
terraform {
  backend "s3" {
    bucket   = "example-iac-tfstate"
    role_arn = "arn:aws:iam::732277778391:role/dr-deployer"
  }
}
`), 0o644)

	got := ExtractBackendAuth(dir)
	want := BackendAuth{AccountID: "732277778391"}
	if got != want {
		t.Errorf("ExtractBackendAuth = %+v, want %+v", got, want)
	}
}

func TestExtractBackendAuth_ExplicitProfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.tf"), []byte(`
terraform {
  backend "s3" {
    bucket  = "example-iac-tfstate"
    profile = "prod-tfstate"
  }
}

provider "aws" {
  region  = "us-east-1"
  profile = "example-dev"
}
`), 0o644)

	got := ExtractBackendAuth(dir)
	want := BackendAuth{Profile: "prod-tfstate"}
	if got != want {
		t.Errorf("ExtractBackendAuth = %+v, want %+v", got, want)
	}
}

func TestExtractBackendAuth_IgnoresProviderBlock(t *testing.T) {
	// role_arn / profile in the provider block must not be returned —
	// only attributes inside `backend "s3"` count.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.tf"), []byte(`
terraform {
  backend "s3" {
    bucket = "example-iac-tfstate"
    key    = "dev/vpc/terraform.tfstate"
  }
}

provider "aws" {
  region  = "us-east-1"
  profile = "example-dev"
  assume_role { role_arn = "arn:aws:iam::585912155334:role/dev-deployer" }
}
`), 0o644)

	got := ExtractBackendAuth(dir)
	if got != (BackendAuth{}) {
		t.Errorf("ExtractBackendAuth = %+v, want zero (no backend auth)", got)
	}
}

func TestExtractBackendAuth_NoBackend(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.tf"), []byte(`
provider "aws" {
  region = "us-east-1"
}
`), 0o644)

	got := ExtractBackendAuth(dir)
	if got != (BackendAuth{}) {
		t.Errorf("ExtractBackendAuth = %+v, want zero", got)
	}
}

func TestExtractBackendAuth_NonS3Backend(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.tf"), []byte(`
terraform {
  backend "local" {
    path = "terraform.tfstate"
  }
}
`), 0o644)

	got := ExtractBackendAuth(dir)
	if got != (BackendAuth{}) {
		t.Errorf("ExtractBackendAuth = %+v, want zero (non-s3 backend)", got)
	}
}

func TestExtractBackendAuth_MultiLineAssumeRole(t *testing.T) {
	// assume_role block spans multiple lines.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.tf"), []byte(`
terraform {
  backend "s3" {
    bucket = "example-iac-tfstate"
    assume_role {
      role_arn = "arn:aws:iam::593671994769:role/tf-runner"
    }
  }
}
`), 0o644)

	got := ExtractBackendAuth(dir)
	want := BackendAuth{AccountID: "593671994769"}
	if got != want {
		t.Errorf("ExtractBackendAuth = %+v, want %+v", got, want)
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
