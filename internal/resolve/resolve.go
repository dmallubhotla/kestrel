// Package resolve owns all "given X, figure out Y" resolution logic.
// Swoop, helm, terraform commands call into this package rather than
// implementing their own resolution.
package resolve

import "github.com/dmallubhotla/kestrel/internal/config"

// AWSProfileForRoot determines the AWS_PROFILE for a terraform root by
// checking, in order:
//  1. Explicit directory→account ID mapping in cfg.Directories
//  2. Account ID from auto-discovery (e.g. allowed_account_ids in .tf files)
//  3. Active target's resolved AWS profile as a fallback
//
// Account IDs are resolved to AWS profile names via cfg.AWS.Accounts.
func AWSProfileForRoot(cfg *config.Config, rootDir string, rootAccountID string, activeTarget string) string {
	if cfg == nil {
		return ""
	}

	// 1. Explicit directory mapping.
	if accountID, ok := cfg.Directories[rootDir]; ok {
		if p := cfg.ResolveAccountProfile(accountID); p != "" {
			return p
		}
	}

	// 2. Auto-discovered account ID on the root.
	if rootAccountID != "" {
		if p := cfg.ResolveAccountProfile(rootAccountID); p != "" {
			return p
		}
	}

	// 3. Active target fallback.
	if activeTarget != "" {
		if resolved, err := cfg.ResolveTarget(activeTarget); err == nil && resolved.AwsProfile != "" {
			return resolved.AwsProfile
		}
	}

	return ""
}

// AccountIDForRoot returns the raw AWS account ID for a terraform root,
// using the same priority as AWSProfileForRoot but returning the account ID
// instead of the profile name.
func AccountIDForRoot(cfg *config.Config, rootDir string, rootAccountID string, activeTarget string) string {
	if cfg == nil {
		return ""
	}

	if accountID, ok := cfg.Directories[rootDir]; ok {
		return accountID
	}

	if rootAccountID != "" {
		return rootAccountID
	}

	if activeTarget != "" {
		if resolved, err := cfg.ResolveTarget(activeTarget); err == nil && resolved.AccountID != "" {
			return resolved.AccountID
		}
	}

	return ""
}
