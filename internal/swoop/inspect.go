package swoop

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ProfileInfo summarizes a discovered profile directory.
type ProfileInfo struct {
	// Name is the profile directory name (e.g. "dev", "prd").
	Name string

	// RootCount is the number of terraform roots under this profile.
	RootCount int

	// AccountIDs are AWS account IDs found in allowed_account_ids within roots.
	AccountIDs []string
}

// InspectProfiles analyzes discovered roots and returns profile summaries
// with extracted AWS account IDs from provider blocks and HCL files.
// baseDir is the discovery root — ancestor scanning stops here.
func InspectProfiles(roots []Root, baseDir string) []ProfileInfo {
	absBase, _ := filepath.Abs(baseDir)

	byProfile := make(map[string]*ProfileInfo)

	for _, r := range roots {
		pi, ok := byProfile[r.Profile]
		if !ok {
			pi = &ProfileInfo{Name: r.Profile}
			byProfile[r.Profile] = pi
		}
		pi.RootCount++

		// Try to extract account IDs from this root's .tf and .hcl files.
		for _, id := range extractAccountIDs(r.AbsPath) {
			if !contains(pi.AccountIDs, id) {
				pi.AccountIDs = append(pi.AccountIDs, id)
			}
		}

		// Walk up ancestor directories (up to baseDir) for HCL account IDs.
		// Centralized repos often define the account ID in a parent terragrunt.hcl.
		for _, id := range extractAncestorAccountIDs(r.AbsPath, absBase) {
			if !contains(pi.AccountIDs, id) {
				pi.AccountIDs = append(pi.AccountIDs, id)
			}
		}
	}

	result := make([]ProfileInfo, 0, len(byProfile))
	for _, pi := range byProfile {
		result = append(result, *pi)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// allowedAccountRe matches allowed_account_ids = ["123456789012"]
var allowedAccountRe = regexp.MustCompile(`allowed_account_ids\s*=\s*\["(\d{12})"\]`)

// hclAccountRe matches common HCL patterns for account IDs in terragrunt configs:
//
//	aws_account_id = "123456789012"
//	account_id     = "123456789012"
var hclAccountRe = regexp.MustCompile(`(?:aws_)?account_id\s*=\s*"(\d{12})"`)


// extractAccountIDs scans .tf and .hcl files in a directory for account ID values.
func extractAccountIDs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		isTF := strings.HasSuffix(name, ".tf")
		isHCL := strings.HasSuffix(name, ".hcl") && !strings.HasSuffix(name, ".lock.hcl")
		if !isTF && !isHCL {
			continue
		}

		ids = appendAccountIDsFromFile(filepath.Join(dir, name), ids)
	}
	return ids
}

// extractAncestorAccountIDs walks up from dir to stopAt, scanning HCL files
// in each ancestor for account IDs. Stops at stopAt (inclusive).
func extractAncestorAccountIDs(dir, stopAt string) []string {
	var ids []string
	current := filepath.Dir(dir) // start from parent (dir itself was already scanned)

	for {
		// Scan HCL files (not .tf — ancestors aren't terraform roots).
		entries, err := os.ReadDir(current)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if strings.HasSuffix(name, ".hcl") && !strings.HasSuffix(name, ".lock.hcl") {
					ids = appendAccountIDsFromFile(filepath.Join(current, name), ids)
				}
			}
		}

		if current == stopAt || current == filepath.Dir(current) {
			break
		}
		current = filepath.Dir(current)
	}
	return ids
}

// appendAccountIDsFromFile scans a single file for account ID patterns,
// appending any new IDs to the provided slice.
//
// appendAccountIDsFromFile scans a single file for account ID patterns,
// appending any new IDs to the provided slice.
func appendAccountIDsFromFile(path string, ids []string) []string {
	f, err := os.Open(path)
	if err != nil {
		return ids
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := allowedAccountRe.FindStringSubmatch(line); len(m) > 1 {
			if !contains(ids, m[1]) {
				ids = append(ids, m[1])
			}
		}
		if m := hclAccountRe.FindStringSubmatch(line); len(m) > 1 {
			if !contains(ids, m[1]) {
				ids = append(ids, m[1])
			}
		}
	}
	return ids
}

// EnrichWithAccountIDs sets the AccountID field on each root by scanning
// the root's own .tf/.hcl files for account IDs. If the root itself has
// no account IDs, falls back to the profile-level account IDs.
func EnrichWithAccountIDs(roots []Root, profiles []ProfileInfo) {
	// Build profile-level fallback map.
	byProfile := make(map[string]string)
	for _, p := range profiles {
		if len(p.AccountIDs) > 0 {
			byProfile[p.Name] = p.AccountIDs[0]
		}
	}

	for i := range roots {
		// Try the root's own directory first.
		ids := extractAccountIDs(roots[i].AbsPath)
		if len(ids) > 0 {
			roots[i].AccountID = ids[0]
			continue
		}
		// Fall back to profile-level (useful for centralized repos where
		// account IDs are in ancestor dirs, already captured by InspectProfiles).
		if id, ok := byProfile[roots[i].Profile]; ok {
			roots[i].AccountID = id
		}
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
