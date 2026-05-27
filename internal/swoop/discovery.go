package swoop

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Discover walks baseDir and returns all terraform root directories found.
// A terraform root is a directory containing at least one .tf file with a
// terraform { ... backend block. Directories named ".terraform" or "modules"
// are skipped.
func Discover(baseDir string) ([]Root, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}

	var roots []Root

	err = filepath.WalkDir(absBase, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		name := d.Name()
		// Skip hidden dirs (especially .terraform), and common non-root dirs.
		if strings.HasPrefix(name, ".") || name == "modules" || name == "diagrams" {
			return filepath.SkipDir
		}

		if hasTerraformBackend(path) {
			rel, _ := filepath.Rel(absBase, path)
			root := Root{
				Path:        rel,
				AbsPath:     path,
				Dir:         extractDir(rel),
				TFVersion:   readTFVersion(path),
				Initialized: isInitialized(path),
			}
			roots = append(roots, root)
			// Continue walking — nested roots (children with their own
			// backend blocks) are legitimate and should be discovered.
			// .terraform/ and modules/ are already pruned above.
			return nil
		}

		return nil
	})

	return roots, err
}

// hasTerraformBackend returns true if the directory contains a .tf file with
// a backend configuration block. We scan for the pattern `backend "` which
// covers `backend "s3"`, `backend "gcs"`, etc.
func hasTerraformBackend(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		if fileContainsBackend(filepath.Join(dir, e.Name())) {
			return true
		}
	}
	return false
}

// fileContainsBackend does a simple line scan for a backend block indicator.
// This avoids pulling in a full HCL parser while being reliable enough for
// well-structured terraform files.
func fileContainsBackend(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "backend ") && strings.Contains(line, "\"") {
			return true
		}
	}
	return false
}

// extractDir returns the first path component — the top-level directory
// (e.g. "dev", "prd", "dr").
func extractDir(relPath string) string {
	parts := strings.SplitN(relPath, string(os.PathSeparator), 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// readTFVersion reads the .terraform-version file in dir if it exists.
func readTFVersion(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".terraform-version"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// isInitialized returns true if the directory contains a .terraform subdirectory.
func isInitialized(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".terraform"))
	return err == nil && info.IsDir()
}

// latestTFMtime returns the most recent modification time of any .tf file in dir.
func latestTFMtime(dir string) time.Time {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}
	}
	var latest time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

// EnrichTFMtimes sets TFModified on each root to the most recent .tf file mtime.
func EnrichTFMtimes(roots []Root) {
	for i := range roots {
		roots[i].TFModified = latestTFMtime(roots[i].AbsPath)
	}
}
