package swoop

import (
	"path/filepath"
	"sort"
	"testing"
)

func makeRoots() []Root {
	return []Root{
		{Path: filepath.Join("dev", "networking", "vpc"), Profile: "dev"},
		{Path: filepath.Join("dev", "networking", "route53"), Profile: "dev"},
		{Path: filepath.Join("dev", "data-stores", "fhr-db"), Profile: "dev"},
		{Path: filepath.Join("prd", "us-east-1", "prod", "vpc"), Profile: "prd"},
		{Path: filepath.Join("prd", "us-east-1", "stage", "vpc"), Profile: "prd"},
		{Path: filepath.Join("dr", "networking", "vpc"), Profile: "dr"},
	}
}

func TestResolve_ExactMatch(t *testing.T) {
	roots := makeRoots()
	target := filepath.Join("dev", "networking", "vpc")
	matches := Resolve(roots, target)

	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Path != target {
		t.Errorf("got %q, want %q", matches[0].Path, target)
	}
}

func TestResolve_Substring(t *testing.T) {
	roots := makeRoots()
	matches := Resolve(roots, "fhr-db")

	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Path != filepath.Join("dev", "data-stores", "fhr-db") {
		t.Errorf("got %q", matches[0].Path)
	}
}

func TestResolve_SubstringMultiple(t *testing.T) {
	roots := makeRoots()
	matches := Resolve(roots, "vpc")

	// Should match all roots containing "vpc".
	if len(matches) != 4 {
		t.Fatalf("got %d matches, want 4: %v", len(matches), matchPaths(matches))
	}
}

func TestResolve_CaseInsensitive(t *testing.T) {
	roots := makeRoots()
	matches := Resolve(roots, "VPC")
	if len(matches) != 4 {
		t.Fatalf("got %d matches, want 4", len(matches))
	}
}

func TestResolve_EmptyTarget(t *testing.T) {
	roots := makeRoots()
	matches := Resolve(roots, "")
	if len(matches) != len(roots) {
		t.Fatalf("got %d matches, want %d", len(matches), len(roots))
	}
}

func TestResolve_NoMatch(t *testing.T) {
	roots := makeRoots()
	matches := Resolve(roots, "nonexistent")
	if len(matches) != 0 {
		t.Fatalf("got %d matches, want 0", len(matches))
	}
}

func TestResolveByProfile(t *testing.T) {
	roots := makeRoots()
	matches := ResolveByProfile(roots, "dev")

	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3", len(matches))
	}
	for _, m := range matches {
		if m.Profile != "dev" {
			t.Errorf("got profile %q, want %q", m.Profile, "dev")
		}
	}
}

func matchPaths(roots []Root) []string {
	out := make([]string, len(roots))
	for i, r := range roots {
		out[i] = r.Path
	}
	sort.Strings(out)
	return out
}
