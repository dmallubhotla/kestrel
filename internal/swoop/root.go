package swoop

import "time"

// Root represents a discovered terraform root directory.
type Root struct {
	// Path is the root's path relative to the discovery base directory.
	Path string

	// AbsPath is the absolute filesystem path.
	AbsPath string

	// Profile is the inferred account profile (top-level directory name).
	// For service-embedded IaC this may be the environment name (e.g. "dev").
	Profile string

	// AccountID is the AWS account ID discovered from terraform files.
	// Set by EnrichWithAccountIDs after discovery + inspection.
	AccountID string

	// TFVersion is the terraform version from .terraform-version, if present.
	TFVersion string

	// Initialized is true when a .terraform/ directory exists in the root.
	Initialized bool

	// GitDirty is true when the root contains uncommitted changes.
	// Set by EnrichGitStatus after discovery.
	GitDirty bool

	// TFModified is the most recent modification time of any .tf file in the root.
	// Set by EnrichTFMtimes after discovery.
	TFModified time.Time
}

// StateEntry records when terraform actions were last run against a root.
type StateEntry struct {
	LastInit   *time.Time `yaml:"last_init,omitempty"`
	LastPlan   *time.Time `yaml:"last_plan,omitempty"`
	LastApply  *time.Time `yaml:"last_apply,omitempty"`
	PlanResult string     `yaml:"plan_result,omitempty"`
}
