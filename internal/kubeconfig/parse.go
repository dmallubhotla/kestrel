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

// ReadContexts parses ~/.kube/config and returns all context entries.
func ReadContexts() ([]Context, error) {
	path := kubeConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return ParseContexts(data)
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

func kubeConfigPath() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		// Take the first path if multiple are specified.
		if i := strings.IndexByte(v, ':'); i >= 0 {
			return v[:i]
		}
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kube", "config")
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
