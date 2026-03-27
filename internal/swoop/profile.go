package swoop

import "github.com/example/kestrel/internal/config"

// ResolveAWSProfile determines the AWS_PROFILE for a root by checking:
//  1. The root's Profile field against config environments (direct name match)
//  2. Account ID fallback: if the root's environment has an aws_account_id,
//     find any other environment with the same account ID that has a profile
//  3. Falls back to the active environment's AWS profile if provided
func ResolveAWSProfile(root Root, cfg *config.Config, activeEnv string) string {
	if cfg == nil {
		return ""
	}

	// 1. Direct name match.
	if env, ok := cfg.Environments[root.Profile]; ok && env.AwsProfile != "" {
		return env.AwsProfile
	}

	// 2. Account ID fallback: root's directory has an account ID but no direct
	//    profile — find another environment sharing that account.
	if env, ok := cfg.Environments[root.Profile]; ok && env.AwsAccountID != "" {
		if p := resolveProfileByAccountID(cfg, env.AwsAccountID); p != "" {
			return p
		}
	}

	// 3. Active environment fallback.
	if activeEnv != "" {
		if env, ok := cfg.Environments[activeEnv]; ok && env.AwsProfile != "" {
			return env.AwsProfile
		}
	}

	return ""
}

// resolveProfileByAccountID searches all environments for one that shares the
// given account ID and has an aws_profile configured.
func resolveProfileByAccountID(cfg *config.Config, accountID string) string {
	for _, env := range cfg.Environments {
		if env.AwsAccountID == accountID && env.AwsProfile != "" {
			return env.AwsProfile
		}
	}
	return ""
}
