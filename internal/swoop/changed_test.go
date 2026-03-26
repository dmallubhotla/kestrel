package swoop

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestChangedRoots(t *testing.T) {
	// Set up a git repo with two roots and a commit history.
	base := t.TempDir()

	// Init git repo.
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = base
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")

	// Create two terraform roots.
	createTFRoot(t, filepath.Join(base, "dev", "vpc"))
	createTFRoot(t, filepath.Join(base, "dev", "rds"))

	// Initial commit with both roots.
	git("add", "-A")
	git("commit", "-m", "initial")

	// Modify only the vpc root on a branch.
	git("checkout", "-b", "feature")
	os.WriteFile(filepath.Join(base, "dev", "vpc", "extra.tf"), []byte(`
resource "aws_subnet" "extra" {
  vpc_id = "vpc-123"
}
`), 0o644)
	git("add", "-A")
	git("commit", "-m", "add extra subnet")

	// Discover roots.
	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}

	// Find changed roots vs main.
	changed, err := ChangedRoots(roots, base, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(changed) != 1 {
		t.Fatalf("expected 1 changed root, got %d: %v", len(changed), rootPaths(changed))
	}
	if changed[0].Path != filepath.Join("dev", "vpc") {
		t.Errorf("expected dev/vpc, got %s", changed[0].Path)
	}
}

func TestChangedRoots_ExplicitRef(t *testing.T) {
	base := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = base
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")

	createTFRoot(t, filepath.Join(base, "dev", "vpc"))
	createTFRoot(t, filepath.Join(base, "dev", "rds"))
	git("add", "-A")
	git("commit", "-m", "initial")

	// Modify rds.
	os.WriteFile(filepath.Join(base, "dev", "rds", "extra.tf"), []byte(`
resource "aws_db_instance" "extra" {}
`), 0o644)
	git("add", "-A")
	git("commit", "-m", "modify rds")

	roots, err := Discover(base)
	if err != nil {
		t.Fatal(err)
	}

	// Changed vs HEAD~1.
	changed, err := ChangedRoots(roots, base, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}

	if len(changed) != 1 {
		t.Fatalf("expected 1 changed root, got %d", len(changed))
	}
	if changed[0].Path != filepath.Join("dev", "rds") {
		t.Errorf("expected dev/rds, got %s", changed[0].Path)
	}
}

func TestPathContains(t *testing.T) {
	tests := []struct {
		parent, child string
		want          bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/b/c/d.tf", true},
		{"/a/b", "/a/bc", false},
		{"/a/b", "/a", false},
	}
	for _, tt := range tests {
		got := pathContains(tt.parent, tt.child)
		if got != tt.want {
			t.Errorf("pathContains(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
		}
	}
}
