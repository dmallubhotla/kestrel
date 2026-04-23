package swoop

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DirInfo summarizes a discovered top-level directory.
type DirInfo struct {
	// Name is the directory name (e.g. "dev", "prd").
	Name string

	// RootCount is the number of terraform roots under this directory.
	RootCount int

	// AccountIDs are AWS account IDs found in allowed_account_ids within roots.
	AccountIDs []string
}

// InspectDirs analyzes discovered roots and returns directory summaries
// with extracted AWS account IDs from provider blocks and HCL files.
// baseDir is the discovery root — ancestor scanning stops here.
func InspectDirs(roots []Root, baseDir string) []DirInfo {
	absBase, _ := filepath.Abs(baseDir)

	byDir := make(map[string]*DirInfo)

	for _, r := range roots {
		di, ok := byDir[r.Dir]
		if !ok {
			di = &DirInfo{Name: r.Dir}
			byDir[r.Dir] = di
		}
		di.RootCount++

		// Try to extract account IDs from this root's .tf and .hcl files.
		for _, id := range extractAccountIDs(r.AbsPath) {
			if !contains(di.AccountIDs, id) {
				di.AccountIDs = append(di.AccountIDs, id)
			}
		}

		// Walk up ancestor directories (up to baseDir) for HCL account IDs.
		// Centralized repos often define the account ID in a parent terragrunt.hcl.
		for _, id := range extractAncestorAccountIDs(r.AbsPath, absBase) {
			if !contains(di.AccountIDs, id) {
				di.AccountIDs = append(di.AccountIDs, id)
			}
		}
	}

	result := make([]DirInfo, 0, len(byDir))
	for _, di := range byDir {
		result = append(result, *di)
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

// regionRe matches AWS region declarations in provider blocks:
//
//	region = "us-east-1"
var regionRe = regexp.MustCompile(`region\s*=\s*"([a-z]+-[a-z]+-\d+)"`)

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
// the root's own .tf/.hcl files first, then walking ancestors up to baseDir.
// This avoids directory-level fallback which conflates unrelated roots that
// happen to share a top-level directory (e.g. service repos where all roots
// are under "misc").
func EnrichWithAccountIDs(roots []Root, baseDir string) {
	absBase, _ := filepath.Abs(baseDir)

	for i := range roots {
		// Try the root's own directory first.
		ids := extractAccountIDs(roots[i].AbsPath)
		if len(ids) > 0 {
			roots[i].AccountID = ids[0]
			continue
		}
		// Walk ancestors — useful for centralized repos where account IDs
		// are in a parent terragrunt.hcl.
		ancestorIDs := extractAncestorAccountIDs(roots[i].AbsPath, absBase)
		if len(ancestorIDs) > 0 {
			roots[i].AccountID = ancestorIDs[0]
		}
		// If still nothing, leave empty — the root doesn't need AWS.
	}
}

// ExtractRegion scans .tf files in a directory for an AWS region declaration.
// Returns the first region found, or empty string.
func ExtractRegion(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

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
			if m := regionRe.FindStringSubmatch(scanner.Text()); len(m) > 1 {
				f.Close()
				return m[1]
			}
		}
		f.Close()
	}
	return ""
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
