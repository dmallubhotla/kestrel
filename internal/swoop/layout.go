package swoop

import (
	"path/filepath"
	"strings"
)

// Layout describes the detected IaC repository archetype.
type Layout struct {
	// Type is "service" for service-embedded IaC or "centralized" for
	// dedicated IaC repos.
	Type string

	// IACDir is the inferred terraform base directory relative to the project
	// root (e.g. "misc/iac" for service repos). Empty for centralized repos
	// where the entire repo is IaC.
	IACDir string

	// EnvNames are the discovered environment names (e.g. ["ci","dev","prod","stage"]).
	// For service repos these are the dirs under live/.
	// For centralized repos these are the top-level profile directories.
	EnvNames []string
}

// DetectLayout analyzes discovered roots (relative to the project root, not
// the IaC base dir) to determine the repo archetype.
//
// Service-embedded pattern: roots match <prefix>/live/<env>/ where each root
// is exactly one level under a "live" directory and <prefix> is the IaC dir.
//
// Centralized pattern: roots are organized by top-level account profile
// directories (dev/, prd/, dr/, etc.) with no common "live" parent.
func DetectLayout(roots []Root) Layout {
	// Check if all roots share a common <prefix>/live/<env> pattern.
	var livePrefix string
	allLive := true
	envSet := make(map[string]bool)

	for _, r := range roots {
		parts := strings.Split(r.Path, string(filepath.Separator))
		liveIdx := -1
		for i, p := range parts {
			if p == "live" {
				liveIdx = i
				break
			}
		}

		if liveIdx < 0 || liveIdx+1 >= len(parts) {
			allLive = false
			break
		}

		prefix := strings.Join(parts[:liveIdx], string(filepath.Separator))
		if livePrefix == "" {
			livePrefix = prefix
		} else if prefix != livePrefix {
			allLive = false
			break
		}

		// The env name is the directory immediately after "live".
		envSet[parts[liveIdx+1]] = true
	}

	if allLive && livePrefix != "" {
		envNames := sortedKeys(envSet)
		return Layout{
			Type:     "service",
			IACDir:   livePrefix,
			EnvNames: envNames,
		}
	}

	// Centralized: environments are the top-level (profile) directories.
	profileSet := make(map[string]bool)
	for _, r := range roots {
		profileSet[r.Profile] = true
	}

	return Layout{
		Type:     "centralized",
		EnvNames: sortedKeys(profileSet),
	}
}

// EnvNameFromRoot extracts the environment/target name for a root.
// For service repos, this is the directory after "live/" in the root path.
// For centralized repos, this is the top-level profile directory.
func (l Layout) EnvNameFromRoot(r Root) string {
	if l.Type == "service" {
		parts := strings.Split(r.Path, string(filepath.Separator))
		for i, p := range parts {
			if p == "live" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
		return ""
	}
	return r.Profile
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort — small sets.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
