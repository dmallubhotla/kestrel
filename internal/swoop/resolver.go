package swoop

import (
	"path/filepath"
	"strings"
)

// Resolve takes a target string and returns matching roots.
// Matching strategies (in order):
//  1. Exact relative path match
//  2. Glob pattern match (if target contains * or ?)
//  3. Substring match (target appears anywhere in the root path)
func Resolve(roots []Root, target string) []Root {
	if target == "" {
		return roots
	}

	// 1. Exact match
	for _, r := range roots {
		if r.Path == target || r.Path == target+"/" {
			return []Root{r}
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
			return matches
		}
	}

	// 3. Substring / fuzzy match
	lower := strings.ToLower(target)
	var matches []Root
	for _, r := range roots {
		if strings.Contains(strings.ToLower(r.Path), lower) {
			matches = append(matches, r)
		}
	}
	return matches
}

// ResolveByProfile returns all roots under the given profile directory.
func ResolveByProfile(roots []Root, profile string) []Root {
	var matches []Root
	for _, r := range roots {
		if r.Profile == profile {
			matches = append(matches, r)
		}
	}
	return matches
}
