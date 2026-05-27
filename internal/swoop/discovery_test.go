package swoop

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// helper to create a minimal terraform root with a backend block.
func createTFRoot(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `terraform {
  backend "s3" {
    bucket = "my-bucket"
    key    = "test/terraform.tfstate"
  }
}

provider "aws" {
  region = "us-east-1"
}
`
	if err := os.WriteFile(filepath.Join(dir, "root.tf"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover_BasicStructure(t *testing.T) {
	base := t.TempDir()

	// Create a few roots mimicking iac-live layout.
	createTFRoot(t, filepath.Join(base, "dev", "networking", "vpc"))
	createTFRoot(t, filepath.Join(base, "dev", "data-stores", "fhr-db"))
	createTFRoot(t, filepath.Join(base, "prd", "us-east-1", "prod", "vpc"))

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	paths := rootPaths(roots)
	sort.Strings(paths)

	expected := []string{
		filepath.Join("dev", "data-stores", "fhr-db"),
		filepath.Join("dev", "networking", "vpc"),
		filepath.Join("prd", "us-east-1", "prod", "vpc"),
	}
	if len(paths) != len(expected) {
		t.Fatalf("got %d roots, want %d: %v", len(paths), len(expected), paths)
	}
	for i, p := range expected {
		if paths[i] != p {
			t.Errorf("root[%d] = %q, want %q", i, paths[i], p)
		}
	}
}

func TestDiscover_DirExtraction(t *testing.T) {
	base := t.TempDir()

	createTFRoot(t, filepath.Join(base, "dev", "vpc"))
	createTFRoot(t, filepath.Join(base, "prd", "vpc"))
	createTFRoot(t, filepath.Join(base, "dr", "vpc"))

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	dirs := make(map[string]string)
	for _, r := range roots {
		dirs[r.Path] = r.Dir
	}

	want := map[string]string{
		filepath.Join("dev", "vpc"): "dev",
		filepath.Join("prd", "vpc"): "prd",
		filepath.Join("dr", "vpc"):  "dr",
	}
	for path, wantDir := range want {
		if got := dirs[path]; got != wantDir {
			t.Errorf("dir for %q = %q, want %q", path, got, wantDir)
		}
	}
}

func TestDiscover_TFVersionReading(t *testing.T) {
	base := t.TempDir()

	root := filepath.Join(base, "dev", "vpc")
	createTFRoot(t, root)
	if err := os.WriteFile(filepath.Join(root, ".terraform-version"), []byte("1.9.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1", len(roots))
	}
	if roots[0].TFVersion != "1.9.2" {
		t.Errorf("TFVersion = %q, want %q", roots[0].TFVersion, "1.9.2")
	}
}

func TestDiscover_InitDetection(t *testing.T) {
	base := t.TempDir()

	root := filepath.Join(base, "dev", "vpc")
	createTFRoot(t, root)

	// Not initialized yet.
	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}
	if roots[0].Initialized {
		t.Error("root should not be initialized")
	}

	// Create .terraform dir.
	if err := os.MkdirAll(filepath.Join(root, ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}

	roots, err = Discover(base)
	if err != nil {
		t.Fatal(err)
	}
	if !roots[0].Initialized {
		t.Error("root should be initialized")
	}
}

func TestDiscover_SkipsDotDirs(t *testing.T) {
	base := t.TempDir()

	createTFRoot(t, filepath.Join(base, "dev", "vpc"))
	// This should be skipped — it's inside a .terraform directory.
	createTFRoot(t, filepath.Join(base, "dev", "vpc", ".terraform", "modules", "foo"))
	// This should be skipped — hidden directory.
	createTFRoot(t, filepath.Join(base, ".hidden", "vpc"))

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1: %v", len(roots), rootPaths(roots))
	}
	if roots[0].Path != filepath.Join("dev", "vpc") {
		t.Errorf("got %q, want %q", roots[0].Path, filepath.Join("dev", "vpc"))
	}
}

func TestDiscover_SkipsDirsWithoutBackend(t *testing.T) {
	base := t.TempDir()

	// A directory with .tf files but no backend block should not be a root.
	noBackend := filepath.Join(base, "dev", "misc")
	if err := os.MkdirAll(noBackend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noBackend, "main.tf"), []byte(`
provider "aws" {
  region = "us-east-1"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	createTFRoot(t, filepath.Join(base, "dev", "vpc"))

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1: %v", len(roots), rootPaths(roots))
	}
}

func TestDiscover_ServiceEmbeddedLayout(t *testing.T) {
	base := t.TempDir()

	// Service repo layout: misc/iac/live/{env}/
	createTFRoot(t, filepath.Join(base, "live", "dev"))
	createTFRoot(t, filepath.Join(base, "live", "stage"))
	createTFRoot(t, filepath.Join(base, "live", "prod"))

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	if len(roots) != 3 {
		t.Fatalf("got %d roots, want 3: %v", len(roots), rootPaths(roots))
	}

	// Dir should be "live" for all (first path component).
	for _, r := range roots {
		if r.Dir != "live" {
			t.Errorf("dir for %q = %q, want %q", r.Path, r.Dir, "live")
		}
	}
}

func TestDiscover_NestedRoots(t *testing.T) {
	base := t.TempDir()

	// Parent root with backend.
	createTFRoot(t, filepath.Join(base, "prd", "_global", "github-oidc"))
	// Child root nested inside parent — has its own backend.
	createTFRoot(t, filepath.Join(base, "prd", "_global", "github-oidc", "geodata-store-api"))
	// Unrelated sibling root.
	createTFRoot(t, filepath.Join(base, "dev", "networking", "vpc"))

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	paths := rootPaths(roots)
	sort.Strings(paths)

	expected := []string{
		filepath.Join("dev", "networking", "vpc"),
		filepath.Join("prd", "_global", "github-oidc"),
		filepath.Join("prd", "_global", "github-oidc", "geodata-store-api"),
	}
	if len(paths) != len(expected) {
		t.Fatalf("got %d roots, want %d: %v", len(paths), len(expected), paths)
	}
	for i, p := range expected {
		if paths[i] != p {
			t.Errorf("root[%d] = %q, want %q", i, paths[i], p)
		}
	}
}

func rootPaths(roots []Root) []string {
	out := make([]string, len(roots))
	for i, r := range roots {
		out[i] = r.Path
	}
	return out
}
