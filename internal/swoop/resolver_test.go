package swoop

import (
	"path/filepath"
	"sort"
	"testing"
)

func makeRoots() []Root {
	return []Root{
		{Path: filepath.Join("dev", "networking", "vpc"), Dir: "dev"},
		{Path: filepath.Join("dev", "networking", "route53"), Dir: "dev"},
		{Path: filepath.Join("dev", "data-stores", "fhr-db"), Dir: "dev"},
		{Path: filepath.Join("prd", "us-east-1", "prod", "vpc"), Dir: "prd"},
		{Path: filepath.Join("prd", "us-east-1", "stage", "vpc"), Dir: "prd"},
		{Path: filepath.Join("dr", "networking", "vpc"), Dir: "dr"},
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

func TestResolveByDir(t *testing.T) {
	roots := makeRoots()
	matches := ResolveByDir(roots, "dev")

	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3", len(matches))
	}
	for _, m := range matches {
		if m.Dir != "dev" {
			t.Errorf("got dir %q, want %q", m.Dir, "dev")
		}
	}
}

func TestResolve_FuzzySegment(t *testing.T) {
	roots := []Root{
		{Path: filepath.Join("dev", "services", "gha-runners"), Dir: "dev"},
		{Path: filepath.Join("dev", "networking", "vpc"), Dir: "dev"},
		{Path: filepath.Join("prd", "services", "gha-runners"), Dir: "prd"},
		{Path: filepath.Join("dev", "data-stores", "fhr-db"), Dir: "dev"},
	}

	// "dev/gha" should NOT match via substring (literal "dev/gha" not in path),
	// but SHOULD match via fuzzy segments: "dev" → "dev", "gha" → "gha-runners".
	matches := Resolve(roots, filepath.Join("dev", "gha"))
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1: %v", len(matches), matchPaths(matches))
	}
	if matches[0].Path != filepath.Join("dev", "services", "gha-runners") {
		t.Errorf("got %q", matches[0].Path)
	}
}

func TestResolve_FuzzySegmentMultiple(t *testing.T) {
	roots := []Root{
		{Path: filepath.Join("dev", "services", "gha-runners"), Dir: "dev"},
		{Path: filepath.Join("prd", "services", "gha-runners"), Dir: "prd"},
		{Path: filepath.Join("dev", "networking", "vpc"), Dir: "dev"},
	}

	// "gha" alone matches via substring, but "services/gha" should fuzzy-match both.
	matches := Resolve(roots, filepath.Join("services", "gha"))
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2: %v", len(matches), matchPaths(matches))
	}
}

func TestResolve_FuzzySegmentNoMatch(t *testing.T) {
	roots := makeRoots()
	// "prd/route53" — prd has no route53 root.
	matches := Resolve(roots, filepath.Join("prd", "route53"))
	if len(matches) != 0 {
		t.Fatalf("got %d matches, want 0: %v", len(matches), matchPaths(matches))
	}
}

func TestResolve_FuzzySegmentOrder(t *testing.T) {
	roots := []Root{
		{Path: filepath.Join("alpha", "beta", "gamma"), Dir: "dev"},
	}
	// Segments must match in order: "gamma/alpha" should not match.
	matches := Resolve(roots, filepath.Join("gamma", "alpha"))
	if len(matches) != 0 {
		t.Fatalf("got %d matches, want 0: %v", len(matches), matchPaths(matches))
	}
}

func TestResolve_FuzzySegmentCaseInsensitive(t *testing.T) {
	roots := []Root{
		{Path: filepath.Join("Dev", "Services", "GHA-Runners"), Dir: "dev"},
	}
	matches := Resolve(roots, filepath.Join("dev", "gha"))
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1: %v", len(matches), matchPaths(matches))
	}
}

func TestResolveWithType_ExactReturnsMatchExact(t *testing.T) {
	roots := makeRoots()
	target := filepath.Join("dev", "networking", "vpc")
	matches, mt := ResolveWithType(roots, target)
	if mt != MatchExact {
		t.Errorf("got match type %d, want MatchExact (%d)", mt, MatchExact)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
}

func TestResolveWithType_SubstringReturnsMatchSubstring(t *testing.T) {
	roots := makeRoots()
	_, mt := ResolveWithType(roots, "vpc")
	if mt != MatchSubstring {
		t.Errorf("got match type %d, want MatchSubstring (%d)", mt, MatchSubstring)
	}
}

func TestResolveWithType_FuzzyReturnsMatchFuzzy(t *testing.T) {
	roots := []Root{
		{Path: filepath.Join("dev", "services", "gha-runners"), Dir: "dev"},
	}
	matches, mt := ResolveWithType(roots, filepath.Join("dev", "gha"))
	if mt != MatchFuzzy {
		t.Errorf("got match type %d, want MatchFuzzy (%d)", mt, MatchFuzzy)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
}

func TestResolveWithType_EmptyReturnsMatchAll(t *testing.T) {
	roots := makeRoots()
	matches, mt := ResolveWithType(roots, "")
	if mt != MatchAll {
		t.Errorf("got match type %d, want MatchAll (%d)", mt, MatchAll)
	}
	if len(matches) != len(roots) {
		t.Fatalf("got %d matches, want %d", len(matches), len(roots))
	}
}

func TestResolve_SubstringPreferredOverFuzzy(t *testing.T) {
	// When a target matches via substring, it should NOT be reported as fuzzy.
	roots := makeRoots()
	_, mt := ResolveWithType(roots, "fhr-db")
	if mt != MatchSubstring {
		t.Errorf("got match type %d, want MatchSubstring (%d)", mt, MatchSubstring)
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
