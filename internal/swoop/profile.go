package swoop

import "github.com/example/kestrel/internal/config"

// ResolveAWSProfile determines the AWS_PROFILE for a root by checking:
// 1. The root's Profile field against config environments
// 2. Falls back to the active environment's AWS profile if provided
func ResolveAWSProfile(root Root, cfg *config.Config, activeEnv string) string {
	if cfg == nil {
		return ""
	}

	// Try matching the root's profile directory to a configured environment.
	if env, ok := cfg.Environments[root.Profile]; ok && env.AwsProfile != "" {
		return env.AwsProfile
	}

	// Fall back to the active environment's profile.
	if activeEnv != "" {
		if env, ok := cfg.Environments[activeEnv]; ok && env.AwsProfile != "" {
			return env.AwsProfile
		}
	}

	return ""
}
