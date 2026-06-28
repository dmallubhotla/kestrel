package swoop

import (
	"github.com/deepak-science/kestrel/internal/config"
	"github.com/deepak-science/kestrel/internal/resolve"
)

// ResolveAWSProfile delegates to resolve.AWSProfileForRoot.
// Deprecated: callers should use resolve.AWSProfileForRoot directly.
func ResolveAWSProfile(root Root, cfg *config.Config, activeTarget string) string {
	return resolve.AWSProfileForRoot(cfg, root.Dir, root.AccountID, activeTarget)
}
