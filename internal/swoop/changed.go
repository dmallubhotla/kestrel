package swoop

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedRoots returns the subset of roots that contain .tf files modified
// relative to the given git ref. If ref is empty, it defaults to the merge-base
// with main (or master).
func ChangedRoots(roots []Root, baseDir, ref string) ([]Root, error) {
	if ref == "" {
		var err error
		ref, err = mergeBase(baseDir)
		if err != nil {
			return nil, err
		}
	}

	changedFiles, err := gitDiffFiles(baseDir, ref)
	if err != nil {
		return nil, err
	}

	return matchChangedFiles(roots, baseDir, changedFiles), nil
}

// matchChangedFiles is the pure logic: given a list of changed file paths
// (relative to baseDir), return the roots that contain changed .tf files.
func matchChangedFiles(roots []Root, baseDir string, changedFiles []string) []Root {
	changedRootPaths := make(map[string]bool)
	for _, f := range changedFiles {
		if !strings.HasSuffix(f, ".tf") {
			continue
		}
		dir := filepath.Dir(f)
		for _, r := range roots {
			if pathContains(r.AbsPath, filepath.Join(baseDir, dir)) {
				changedRootPaths[r.Path] = true
			}
		}
	}

	var result []Root
	for _, r := range roots {
		if changedRootPaths[r.Path] {
			result = append(result, r)
		}
	}
	return result
}

// mergeBase finds the merge-base between HEAD and main (or master).
func mergeBase(dir string) (string, error) {
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "merge-base", "HEAD", branch)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	return "HEAD", nil
}

// gitDiffFiles returns the list of changed file paths (relative to repo root)
// between the given ref and the working tree.
func gitDiffFiles(dir, ref string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// pathContains returns true if child is inside or equal to parent.
func pathContains(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if child == parent {
		return true
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}
