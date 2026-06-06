package swoop

import "time"

// Root represents a discovered terraform root directory.
type Root struct {
	// Path is the root's path relative to the discovery base directory.
	Path string

	// AbsPath is the absolute filesystem path.
	AbsPath string

	// Dir is the top-level directory name (e.g. "dev", "prd").
	// For centralized IaC repos this typically maps to an AWS account.
	Dir string

	// AccountID is the AWS account ID discovered from terraform files.
	// Set by EnrichWithAccountIDs after discovery + inspection.
	AccountID string

	// TFVersion is the version from the version-pin file
	// (.opentofu-version or .terraform-version) in the root, if present.
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
