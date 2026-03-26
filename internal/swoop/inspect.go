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
// with extracted AWS account IDs from provider blocks.
func InspectProfiles(roots []Root) []ProfileInfo {
	byProfile := make(map[string]*ProfileInfo)

	for _, r := range roots {
		pi, ok := byProfile[r.Profile]
		if !ok {
			pi = &ProfileInfo{Name: r.Profile}
			byProfile[r.Profile] = pi
		}
		pi.RootCount++

		// Try to extract account IDs from this root's .tf files.
		for _, id := range extractAccountIDs(r.AbsPath) {
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

// extractAccountIDs scans .tf files in a directory for allowed_account_ids values.
func extractAccountIDs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}

		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if m := allowedAccountRe.FindStringSubmatch(scanner.Text()); len(m) > 1 {
				if !contains(ids, m[1]) {
					ids = append(ids, m[1])
				}
			}
		}
		f.Close()
	}
	return ids
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
