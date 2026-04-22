package swoop

import (
	"path/filepath"
	"strings"
)

// MatchType describes which resolution strategy produced the result.
type MatchType int

const (
	// MatchAll means no target was given — all roots returned.
	MatchAll MatchType = iota
	// MatchExact means the target was an exact path match.
	MatchExact
	// MatchGlob means the target contained glob characters and matched.
	MatchGlob
	// MatchSubstring means the target appeared as a literal substring.
	MatchSubstring
	// MatchFuzzy means segment-aware fuzzy matching was used.
	MatchFuzzy
)

// Resolve takes a target string and returns matching roots.
// Matching strategies (in order):
//  1. Exact relative path match
//  2. Glob pattern match (if target contains * or ?)
//  3. Substring match (target appears anywhere in the root path)
//  4. Fuzzy segment match (each target segment matches a root segment as substring, in order)
func Resolve(roots []Root, target string) []Root {
	matches, _ := ResolveWithType(roots, target)
	return matches
}

// ResolveWithType is like Resolve but also returns which matching strategy
// produced the result. Callers can use this to decide whether to prompt for
// confirmation (e.g. fuzzy matches).
func ResolveWithType(roots []Root, target string) ([]Root, MatchType) {
	if target == "" {
		return roots, MatchAll
	}

	// 1. Exact match
	for _, r := range roots {
		if r.Path == target || r.Path == target+"/" {
			return []Root{r}, MatchExact
		}
	}

	// 2. Glob match
	if strings.ContainsAny(target, "*?[") {
		var matches []Root
		for _, r := range roots {
			if matched, _ := filepath.Match(target, r.Path); matched {
				matches = append(matches, r)
				continue
			}
			// Also try matching against just the deepest component.
			if matched, _ := filepath.Match(target, filepath.Base(r.Path)); matched {
				matches = append(matches, r)
			}
		}
		if len(matches) > 0 {
			return matches, MatchGlob
		}
	}

	// 3. Substring match
	lower := strings.ToLower(target)
	var matches []Root
	for _, r := range roots {
		if strings.Contains(strings.ToLower(r.Path), lower) {
			matches = append(matches, r)
		}
	}
	if len(matches) > 0 {
		return matches, MatchSubstring
	}

	// 4. Fuzzy segment match
	for _, r := range roots {
		if fuzzySegmentMatch(target, r.Path) {
			matches = append(matches, r)
		}
	}
	if len(matches) > 0 {
		return matches, MatchFuzzy
	}

	return nil, MatchSubstring
}

// fuzzySegmentMatch checks whether every segment of target appears as a
// case-insensitive substring of a corresponding root path segment, preserving
// order. Segments are split on filepath.Separator.
//
// Example: target "dev/gha" matches root "dev/services/gha-runners" because
// "dev" ⊂ "dev" (index 0) and "gha" ⊂ "gha-runners" (index 2 > 0).
func fuzzySegmentMatch(target, rootPath string) bool {
	targetSegs := strings.Split(strings.ToLower(target), string(filepath.Separator))
	rootSegs := strings.Split(strings.ToLower(rootPath), string(filepath.Separator))

	if len(targetSegs) == 0 || len(targetSegs) > len(rootSegs) {
		return false
	}

	ri := 0
	for _, ts := range targetSegs {
		found := false
		for ri < len(rootSegs) {
			if strings.Contains(rootSegs[ri], ts) {
				ri++
				found = true
				break
			}
			ri++
		}
		if !found {
			return false
		}
	}
	return true
}

// ResolveByDir returns all roots under the given top-level directory.
func ResolveByDir(roots []Root, dir string) []Root {
	var matches []Root
	for _, r := range roots {
		if r.Dir == dir {
			matches = append(matches, r)
		}
	}
	return matches
}
