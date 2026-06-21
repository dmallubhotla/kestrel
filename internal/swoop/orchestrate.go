package swoop

import (
	"fmt"
	"io"
	"os"

	"github.com/dmallubhotla/kestrel/internal/config"
	"github.com/dmallubhotla/kestrel/internal/resolve"
)

// Resolution is the resolved credential picture for a terraform root: the
// provider/target AWS profile, the (different) profile its S3 backend needs,
// and the effective profile that should source credentials. This is the one
// resolver shared by `kest swoop` and `kestci` so the two never diverge —
// "works via swoop locally" must predict "works in kestci".
type Resolution struct {
	// ProviderProfile is the profile from account/target/directory mapping
	// (resolve.AWSProfileForRoot). Empty when nothing maps the root.
	ProviderProfile string
	// BackendProfile is the profile a root's S3 backend needs (from its
	// assume_role / profile attribute) when it differs from ProviderProfile.
	// Empty when the backend block has no auth, or it matches ProviderProfile.
	BackendProfile string
	// Effective is the profile that should source credentials: ProviderProfile,
	// falling back to BackendProfile when no provider profile resolved.
	Effective string
	// AccountID is the AWS account the root is expected to act in, for echoing
	// under ambient (CI) credentials so a mismatch fails legibly.
	AccountID string
}

// EffectiveProfiles resolves the credential picture for a root. environment is
// the active `-e` target (may be empty). It combines resolve.AWSProfileForRoot
// (account/target/directory mapping) with the root's S3 backend auth
// (ExtractBackendAuth), mirroring exactly what `kest swoop` applies so
// `kestci` and `kest swoop list` resolve identically.
func EffectiveProfiles(cfg *config.Config, root Root, environment string) Resolution {
	provider := resolve.AWSProfileForRoot(cfg, root.Dir, root.AccountID, environment)

	auth := ExtractBackendAuth(root.AbsPath)
	backend := ""
	switch {
	case auth.Profile != "":
		backend = auth.Profile
	case auth.AccountID != "" && cfg != nil:
		backend = cfg.ResolveAccountProfile(auth.AccountID)
	}
	// Only surface the backend profile when it's a distinct extra credential.
	if backend == provider {
		backend = ""
	}

	effective := provider
	if effective == "" {
		effective = backend
	}

	accountID := resolve.AccountIDForRoot(cfg, root.Dir, root.AccountID, environment)
	if accountID == "" {
		accountID = auth.AccountID
	}

	return Resolution{
		ProviderProfile: provider,
		BackendProfile:  backend,
		Effective:       effective,
		AccountID:       accountID,
	}
}

// Policy parameterizes how a resolved action is executed. The default
// (zero-value) policy is the `kest swoop` posture: inject the effective
// profile as AWS_PROFILE. Ambient is the `kestci` posture: credentials come
// from the environment (OIDC / env vars), so the resolved profile is echoed
// for legibility but never injected.
type Policy struct {
	// Ambient: don't set AWS_PROFILE; credentials come from the environment.
	Ambient bool
	// PrintContext: print the root/aws/tf context block before running.
	PrintContext bool
	// Out is where the context block is written. Defaults to os.Stderr.
	Out io.Writer
}

// Execute runs one terraform action against a root and records it to local
// state, applying credentials per policy. It is the shared execution spine for
// `kest swoop` and `kestci`: the only differences between the two are carried
// in Policy and in the resolved Resolution, not in forked control flow.
func Execute(cfg *config.Config, root Root, baseDir, action string, res Resolution, pol Policy) (*ExecResult, error) {
	profile := res.Effective
	if pol.Ambient {
		// Credentials come from the environment; never override AWS_PROFILE.
		profile = ""
	}

	if pol.PrintContext {
		printContext(pol.out(), root, res, pol)
	}

	command := "terraform"
	if cfg != nil {
		command = cfg.TerraformCommand()
	}

	result, err := RunTerraform(command, root, profile, action)
	RecordAction(baseDir, root.Path, action, result)
	return result, err
}

func (p Policy) out() io.Writer {
	if p.Out != nil {
		return p.Out
	}
	return os.Stderr
}

func printContext(out io.Writer, root Root, res Resolution, pol Policy) {
	fmt.Fprintf(out, "root:    %s\n", root.Path)
	fmt.Fprintf(out, "dir:     %s\n", root.Dir)

	switch {
	case pol.Ambient:
		// Creds are ambient — echo what we expect so an OIDC/role mismatch is
		// legible instead of a cryptic backend auth error.
		switch {
		case res.AccountID != "" && res.Effective != "":
			fmt.Fprintf(out, "aws:     ambient (expect account %s, profile %s)\n", res.AccountID, res.Effective)
		case res.AccountID != "":
			fmt.Fprintf(out, "aws:     ambient (expect account %s)\n", res.AccountID)
		case res.Effective != "":
			fmt.Fprintf(out, "aws:     ambient (expect profile %s)\n", res.Effective)
		default:
			fmt.Fprintf(out, "aws:     ambient\n")
		}
	case res.Effective != "":
		fmt.Fprintf(out, "aws:     %s\n", res.Effective)
	}

	if root.TFVersion != "" {
		fmt.Fprintf(out, "tf:      %s\n", root.TFVersion)
	}
	fmt.Fprintln(out)
}

// RecordAction records a completed terraform action to local swoop state.
// Failures to load or save state are non-fatal and silently ignored — state
// is a convenience (staleness tracking), not a source of truth, and on an
// ephemeral CI runner it may not persist at all.
func RecordAction(baseDir, rootPath, action string, result *ExecResult) {
	state, err := LoadState(baseDir)
	if err != nil {
		return
	}

	switch action {
	case "init":
		state.RecordInit(rootPath)
	case "plan":
		summary := ""
		if result != nil {
			summary = result.PlanSummary
		}
		state.RecordPlan(rootPath, summary)
	case "apply":
		state.RecordApply(rootPath)
	}

	_ = state.Save()
}
