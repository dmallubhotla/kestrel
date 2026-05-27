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

// Field is a key-value pair from an AWS config profile section.
type Field struct {
	Key   string
	Value string
}

// Profile is a named AWS config profile with its non-sensitive fields.
type Profile struct {
	Name   string
	Fields []Field
}

var sensitiveKeys = map[string]bool{
	"aws_access_key_id":     true,
	"aws_secret_access_key": true,
	"aws_session_token":     true,
}

func awsConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".aws", "config"), nil
}

// ReadProfiles parses ~/.aws/config and returns all named profile entries.
// The "default" profile (if present) is included as "default".
func ReadProfiles() ([]string, error) {
	path, err := awsConfigPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return ParseProfiles(f)
}

// ReadProfileDetails parses ~/.aws/config and returns profiles with their
// non-sensitive configuration fields.
func ReadProfileDetails() ([]Profile, error) {
	path, err := awsConfigPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return ParseProfileDetails(f)
}

// ParseProfiles extracts AWS profile names from an io.Reader containing
// AWS config file content.
func ParseProfiles(r io.Reader) ([]string, error) {
	profiles, err := ParseProfileDetails(r)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	return names, nil
}

// ParseProfileDetails extracts AWS profiles with their non-sensitive fields
// from an io.Reader containing AWS config file content.
func ParseProfileDetails(r io.Reader) ([]Profile, error) {
	var profiles []Profile
	var current *Profile
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle [default] section
		if line == "[default]" {
			if !seen["default"] {
				profiles = append(profiles, Profile{Name: "default"})
				current = &profiles[len(profiles)-1]
				seen["default"] = true
			}
			continue
		}

		// Handle [profile name] sections
		if m := profileRegex.FindStringSubmatch(line); m != nil {
			name := strings.TrimSpace(m[1])
			if !seen[name] {
				profiles = append(profiles, Profile{Name: name})
				current = &profiles[len(profiles)-1]
				seen[name] = true
			}
			continue
		}

		// Key-value pair within a profile section
		if current != nil {
			if key, val, ok := parseKV(line); ok && !sensitiveKeys[key] {
				current.Fields = append(current.Fields, Field{Key: key, Value: val})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading aws config: %w", err)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
}

func parseKV(line string) (string, string, bool) {
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}
