// Package deploy is the cluster-agnostic app-deploy spine shared by
// `kest deploy` and `kestci deploy`. It mirrors the terraform spine in
// internal/swoop: one resolve → execute loop with a pluggable executor
// (helm or kubectl, chosen per config.Deploy) and a small Policy carrying the
// only intended differences between the interactive and CI surfaces.
package deploy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/dmallubhotla/kestrel/internal/config"
	"github.com/dmallubhotla/kestrel/internal/runner"
)

// Deploy actions.
const (
	ActionApply = "apply" // helm upgrade --install / kubectl apply
	ActionDiff  = "diff"  // helm upgrade --dry-run / kubectl diff (read-only)
)

// Resolution is the resolved cluster-access picture for a deploy target: the
// kube context to act in, an optional explicit kubeconfig file, and the AWS
// profile/account (empty for non-AWS clusters like Talos). This is the one
// resolver shared by `kest deploy` and `kestci deploy`.
type Resolution struct {
	// KubeContext is the --kube-context / --context to use. Empty means use
	// the kubeconfig's current-context.
	KubeContext string
	// Kubeconfig is an explicit --kubeconfig path (already resolved against the
	// project root). Empty means the ambient kubeconfig ($KUBECONFIG / default).
	Kubeconfig string
	// AwsProfile is the profile to inject as AWS_PROFILE (under the non-ambient
	// policy). Empty for non-AWS clusters.
	AwsProfile string
	// AccountID is the AWS account expected, echoed under ambient credentials
	// so a mismatch fails legibly.
	AccountID string
}

// Policy parameterizes how a resolved deploy is executed. The zero value is the
// `kest deploy` posture: inject the resolved AWS profile. Ambient is the
// `kestci` posture: credentials and kubeconfig come from the environment.
type Policy struct {
	// Ambient: don't set AWS_PROFILE; credentials come from the environment.
	Ambient bool
	// PrintContext: print the app/target/aws block before running.
	PrintContext bool
	// Out is where the context block is written. Defaults to os.Stderr.
	Out io.Writer
}

func (p Policy) out() io.Writer {
	if p.Out != nil {
		return p.Out
	}
	return os.Stderr
}

// ExecResult captures the outcome of a deploy execution.
type ExecResult struct {
	ExitCode int
}

// Resolve resolves a target name to its cluster-access picture. Unlike
// config.ResolveTarget (EKS-shaped, errors when a cluster has no mapped
// context), this is lenient: a target's cluster is looked up in
// kubernetes.contexts and falls back to the literal value, so a named context
// like Talos's "admin@homelab" works without a contexts entry.
func Resolve(cfg *config.Config, targetName string) (Resolution, error) {
	if cfg == nil {
		return Resolution{}, fmt.Errorf("no config loaded")
	}
	t, ok := cfg.Targets[targetName]
	if !ok {
		return Resolution{}, fmt.Errorf("target %q not configured (check your .kestconfig targets)", targetName)
	}

	var res Resolution
	if t.Cluster != "" {
		ctx := cfg.ResolveClusterContext(t.Cluster)
		if ctx == "" {
			// No mapping — treat the cluster name as the context name directly.
			ctx = t.Cluster
		}
		res.KubeContext = ctx
	}

	if t.Kubeconfig != "" {
		res.Kubeconfig = projectPath(cfg, t.Kubeconfig)
	}

	// AWS profile: explicit account wins, else extract from an EKS ARN context.
	switch {
	case t.AWSAccount != "":
		res.AccountID = t.AWSAccount
		res.AwsProfile = cfg.ResolveAccountProfile(t.AWSAccount)
	case res.KubeContext != "":
		if acct := config.ExtractAccountIDFromARN(res.KubeContext); acct != "" {
			res.AccountID = acct
			res.AwsProfile = cfg.ResolveAccountProfile(acct)
		}
	}

	return res, nil
}

// Execute runs one deploy action, picking the executor from d.Kind() and
// applying credentials per policy. The command runs from the project root so
// chart/manifest/values paths resolve relative to it. extra is passed through
// to the underlying tool.
func Execute(cfg *config.Config, name string, d config.Deploy, res Resolution, action string, pol Policy, extra []string) (*ExecResult, error) {
	if err := d.Validate(name); err != nil {
		return nil, err
	}

	if pol.PrintContext {
		printContext(pol.out(), name, d, res, pol)
	}

	var env map[string]string
	if !pol.Ambient && res.AwsProfile != "" {
		env = map[string]string{"AWS_PROFILE": res.AwsProfile}
	}

	var command string
	var args []string
	switch d.Kind() {
	case config.DeployHelm:
		command = "helm"
		release := d.ReleaseName
		if release == "" {
			release = name
		}
		args = helmArgs(release, d, res, action, extra)
	case config.DeployManifest:
		command = "kubectl"
		args = kubectlArgs(d.Manifests, res, d.Namespace, action == ActionDiff, extra)
	}

	result, err := runner.RunWithOpts(command, args, runner.Options{
		Dir: projectRoot(cfg),
		Env: env,
	})
	out := &ExecResult{ExitCode: result.ExitCode}

	// `kubectl diff` exits 1 when a diff is present — that's the answer, not a
	// failure. Anything higher is a real error.
	if d.Kind() == config.DeployManifest && action == ActionDiff && result.ExitCode == 1 {
		return out, nil
	}
	return out, err
}

// projectRoot returns the directory containing the loaded .kestconfig, or the
// current working directory if no project config was found.
func projectRoot(cfg *config.Config) string {
	if cfg != nil && cfg.Sources.Project != "" {
		return filepath.Dir(cfg.Sources.Project)
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// projectPath resolves a config-relative path against the project root. Absolute
// paths are returned unchanged.
func projectPath(cfg *config.Config, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(projectRoot(cfg), p)
}

func printContext(out io.Writer, name string, d config.Deploy, res Resolution, pol Policy) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(out, format, a...) }

	p("app:     %s (%s)\n", name, d.Kind())
	p("target:  %s\n", d.Target)
	if res.KubeContext != "" {
		p("context: %s\n", res.KubeContext)
	}
	if res.Kubeconfig != "" {
		p("config:  %s\n", res.Kubeconfig)
	}
	switch {
	case pol.Ambient && res.AccountID != "":
		p("aws:     ambient (expect account %s)\n", res.AccountID)
	case pol.Ambient:
		p("aws:     ambient\n")
	case res.AwsProfile != "":
		p("aws:     %s\n", res.AwsProfile)
	}
	p("\n")
}

// sortedSetArgs flattens a --set map into deterministic key=value strings.
func sortedSetArgs(set map[string]string) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+set[k])
	}
	return out
}
