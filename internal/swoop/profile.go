package swoop

import "github.com/example/kestrel/internal/config"

// ResolveAWSProfile determines the AWS_PROFILE for a root by checking:
//  1. Explicit directory→account ID mapping in cfg.Directories
//  2. Account ID from auto-discovery (stored on root by InspectProfiles)
//  3. Falls back to the active target's AWS profile if provided
//
// Account IDs are resolved to AWS profiles via cfg.Accounts.
func ResolveAWSProfile(root Root, cfg *config.Config, activeTarget string) string {
	if cfg == nil {
		return ""
	}

	// 1. Explicit directory mapping.
	if accountID, ok := cfg.Directories[root.Profile]; ok {
		if p := cfg.ResolveAccountProfile(accountID); p != "" {
			return p
		}
	}

	// 2. Auto-discovered account ID on the root.
	if root.AccountID != "" {
		if p := cfg.ResolveAccountProfile(root.AccountID); p != "" {
			return p
		}
	}

	// 3. Active target fallback — resolve target's cluster → context → account.
	if activeTarget != "" {
		if resolved, err := cfg.ResolveTarget(activeTarget); err == nil && resolved.AwsProfile != "" {
			return resolved.AwsProfile
		}
	}

	return ""
}
