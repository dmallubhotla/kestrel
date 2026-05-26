package kubeconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Context represents a kubeconfig context with non-sensitive metadata.
type Context struct {
	Name      string
	Cluster   string
	Namespace string
}

// kubeConfig is the subset of ~/.kube/config we care about.
type kubeConfig struct {
	Contexts []contextEntry `yaml:"contexts"`
}

type contextEntry struct {
	Name    string        `yaml:"name"`
	Context contextDetail `yaml:"context"`
}

type contextDetail struct {
	Cluster   string `yaml:"cluster"`
	Namespace string `yaml:"namespace"`
}

// ReadContexts parses the kubeconfig file(s) and returns all context entries.
// If KUBECONFIG lists multiple files (separated by the OS list separator),
// contexts from all files are concatenated. When the same context name appears
// in more than one file, the first occurrence wins (matching kubectl).
func ReadContexts() ([]Context, error) {
	paths := kubeConfigPaths()
	seen := make(map[string]struct{})
	var merged []Context
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		ctxs, err := ParseContexts(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		for _, c := range ctxs {
			if _, dup := seen[c.Name]; dup {
				continue
			}
			seen[c.Name] = struct{}{}
			merged = append(merged, c)
		}
	}
	return merged, nil
}

// ParseContexts extracts contexts from kubeconfig YAML bytes.
func ParseContexts(data []byte) ([]Context, error) {
	var kc kubeConfig
	if err := yaml.Unmarshal(data, &kc); err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}

	contexts := make([]Context, 0, len(kc.Contexts))
	for _, e := range kc.Contexts {
		contexts = append(contexts, Context{
			Name:      e.Name,
			Cluster:   e.Context.Cluster,
			Namespace: e.Context.Namespace,
		})
	}
	return contexts, nil
}

// ShortName extracts a human-friendly name from a kube context name.
// For EKS ARNs (arn:aws:eks:region:account:cluster/name) it returns the
// cluster name. For everything else it returns the original name.
func ShortName(contextName string) string {
	if strings.Contains(contextName, ":cluster/") {
		if i := strings.LastIndex(contextName, "/"); i >= 0 {
			return contextName[i+1:]
		}
	}
	return contextName
}

// ExtractAccountID pulls the AWS account ID from an EKS ARN context name.
// Returns empty string if the name is not an ARN.
func ExtractAccountID(arnOrName string) string {
	parts := strings.Split(arnOrName, ":")
	if len(parts) >= 5 && parts[0] == "arn" {
		return parts[4]
	}
	return ""
}

func kubeConfigPaths() []string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		raw := filepath.SplitList(v)
		paths := make([]string, 0, len(raw))
		for _, p := range raw {
			if p != "" {
				paths = append(paths, p)
			}
		}
		if len(paths) > 0 {
			return paths
		}
	}
	home, _ := os.UserHomeDir()
	return []string{filepath.Join(home, ".kube", "config")}
}

// BestMatch returns the index of the kube context that best matches the
// given AWS profile name and optional SSO account ID. Returns -1 if no
// match is found.
//
// Matching strategy (in priority order):
//  1. Account ID match — profile's sso_account_id appears in an EKS ARN context name
//  2. Name substring — profile name appears in context name, or vice versa
func BestMatch(profileName string, ssoAccountID string, contexts []Context) int {
	bestIdx := -1
	bestScore := 0

	lower := strings.ToLower(profileName)

	for i, ctx := range contexts {
		ctxLower := strings.ToLower(ctx.Name)

		// Account ID in ARN is the strongest signal.
		if ssoAccountID != "" && strings.Contains(ctxLower, ssoAccountID) {
			score := 10
			// Bonus if profile name also matches (disambiguates multiple
			// clusters in the same account).
			if strings.Contains(ctxLower, lower) || strings.Contains(lower, ctxLower) {
				score += 5
			}
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
			continue
		}

		// Substring match on names.
		if strings.Contains(ctxLower, lower) || strings.Contains(lower, ctxLower) {
			score := 5
			// Exact match is stronger.
			if ctxLower == lower {
				score += 3
			}
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}
	}

	return bestIdx
}
