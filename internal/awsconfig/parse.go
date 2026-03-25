package awsconfig

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var profileRegex = regexp.MustCompile(`^\[profile\s+(.+)\]$`)

// ReadProfiles parses ~/.aws/config and returns all named profile entries.
// The "default" profile (if present) is included as "default".
func ReadProfiles() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determining home directory: %w", err)
	}

	path := filepath.Join(home, ".aws", "config")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	return ParseProfiles(f)
}

// ParseProfiles extracts AWS profile names from an io.Reader containing
// AWS config file content.
func ParseProfiles(r io.Reader) ([]string, error) {
	var profiles []string
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Handle [default] section
		if line == "[default]" {
			if !seen["default"] {
				profiles = append(profiles, "default")
				seen["default"] = true
			}
			continue
		}

		// Handle [profile name] sections
		if m := profileRegex.FindStringSubmatch(line); m != nil {
			name := strings.TrimSpace(m[1])
			if !seen[name] {
				profiles = append(profiles, name)
				seen[name] = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading aws config: %w", err)
	}

	sort.Strings(profiles)
	return profiles, nil
}

// InferEnv tries to detect an environment name from an AWS profile name.
// Returns empty string if no match found.
func InferEnv(profile string) string {
	lower := strings.ToLower(profile)
	switch {
	case strings.Contains(lower, "prod"):
		return "prod"
	case strings.Contains(lower, "stag"):
		return "stage"
	case strings.Contains(lower, "dev"):
		return "dev"
	default:
		return ""
	}
}
