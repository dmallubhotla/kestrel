package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte(`
helm:
  chart: oci://ghcr.io/test/chart:1.0
  values_dir: misc/chart
  release_name: my-app
  namespace: app

targets:
  dev:
    cluster: eks-dev
  prod:
    cluster: eks-prd
  local:
    cluster: kind-local

accounts:
  "585912155334":
    aws_profile: dev-sso
  "593671994769":
    aws_profile: prd-sso

contexts:
  eks-dev: arn:aws:eks:us-east-1:585912155334:cluster/eks-dev
  eks-prd: arn:aws:eks:us-east-1:593671994769:cluster/eks-prd
  kind-local: kind-local

directories:
  prd: "593671994769"
  dev: "585912155334"
`), 0o644)

	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	if cfg.Helm.Chart != "oci://ghcr.io/test/chart:1.0" {
		t.Errorf("chart = %q", cfg.Helm.Chart)
	}

	if len(cfg.Targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(cfg.Targets))
	}
	if cfg.Targets["dev"].Cluster != "eks-dev" {
		t.Errorf("targets[dev].cluster = %q", cfg.Targets["dev"].Cluster)
	}

	if len(cfg.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(cfg.Accounts))
	}
	if cfg.Accounts["585912155334"].AwsProfile != "dev-sso" {
		t.Errorf("accounts[585912155334] = %q", cfg.Accounts["585912155334"].AwsProfile)
	}

	if cfg.Contexts["eks-dev"] != "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev" {
		t.Errorf("contexts[eks-dev] = %q", cfg.Contexts["eks-dev"])
	}

	if cfg.Directories["prd"] != "593671994769" {
		t.Errorf("directories[prd] = %q", cfg.Directories["prd"])
	}
}

func TestCompose_ProjectOverridesUser(t *testing.T) {
	user := &Config{
		Helm: HelmConfig{Chart: "user-chart", Namespace: "user-ns"},
		Accounts: map[string]AccountConfig{
			"585912155334": {AwsProfile: "dev-sso"},
		},
		Contexts: map[string]string{
			"eks-dev": "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
		},
	}
	project := &Config{
		Helm: HelmConfig{Chart: "project-chart", ValuesDir: "misc/chart"},
		Targets: map[string]TargetConfig{
			"dev":  {Cluster: "eks-dev"},
			"prod": {Cluster: "eks-prd"},
		},
		Terraform: TerraformConfig{IACDir: "misc/iac"},
	}

	out := compose(user, project)

	// Project chart overrides user.
	if out.Helm.Chart != "project-chart" {
		t.Errorf("Helm.Chart = %q", out.Helm.Chart)
	}
	// User namespace preserved.
	if out.Helm.Namespace != "user-ns" {
		t.Errorf("Helm.Namespace = %q", out.Helm.Namespace)
	}
	// Targets from project.
	if len(out.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(out.Targets))
	}
	// Accounts from user preserved.
	if out.Accounts["585912155334"].AwsProfile != "dev-sso" {
		t.Errorf("Accounts[585912155334] = %v", out.Accounts["585912155334"])
	}
	// Contexts from user preserved.
	if out.Contexts["eks-dev"] == "" {
		t.Error("Contexts[eks-dev] should be preserved from user")
	}
	// ProjectTargets raw layer.
	if len(out.ProjectTargets) != 2 {
		t.Errorf("ProjectTargets should have 2 entries, got %d", len(out.ProjectTargets))
	}
}

func TestResolveTarget(t *testing.T) {
	cfg := &Config{
		Targets: map[string]TargetConfig{
			"dev":   {Cluster: "eks-dev"},
			"local": {Cluster: "kind-local"},
		},
		Accounts: map[string]AccountConfig{
			"585912155334": {AwsProfile: "dev-sso"},
		},
		Contexts: map[string]string{
			"eks-dev":    "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
			"kind-local": "kind-local",
		},
	}

	// dev: has EKS context → extracts account → resolves profile.
	resolved, err := cfg.ResolveTarget("dev")
	if err != nil {
		t.Fatalf("ResolveTarget(dev): %v", err)
	}
	if resolved.KubeContext != "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev" {
		t.Errorf("dev.KubeContext = %q", resolved.KubeContext)
	}
	if resolved.AwsProfile != "dev-sso" {
		t.Errorf("dev.AwsProfile = %q", resolved.AwsProfile)
	}

	// local: kind context, no AWS.
	resolved, err = cfg.ResolveTarget("local")
	if err != nil {
		t.Fatalf("ResolveTarget(local): %v", err)
	}
	if resolved.KubeContext != "kind-local" {
		t.Errorf("local.KubeContext = %q", resolved.KubeContext)
	}
	if resolved.AwsProfile != "" {
		t.Errorf("local.AwsProfile should be empty, got %q", resolved.AwsProfile)
	}

	// nonexistent: error.
	_, err = cfg.ResolveTarget("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent target")
	}
}

func TestResolveTarget_MissingContext(t *testing.T) {
	cfg := &Config{
		Targets: map[string]TargetConfig{
			"prod": {Cluster: "eks-prd"},
		},
		// No Contexts configured.
	}

	_, err := cfg.ResolveTarget("prod")
	if err == nil {
		t.Fatal("expected error when context not configured")
	}
}

func TestExtractAccountIDFromARN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"arn:aws:eks:us-east-1:585912155334:cluster/eks-dev", "585912155334"},
		{"arn:aws:eks:us-west-2:593671994769:cluster/eks-prd", "593671994769"},
		{"kind-local", ""},
		{"", ""},
		{"arn:aws:eks:us-east-1:short:cluster/x", ""}, // account too short
	}

	for _, tt := range tests {
		got := ExtractAccountIDFromARN(tt.input)
		if got != tt.want {
			t.Errorf("ExtractAccountIDFromARN(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveAccountProfile(t *testing.T) {
	cfg := &Config{
		Accounts: map[string]AccountConfig{
			"585912155334": {AwsProfile: "dev-sso"},
		},
	}

	if got := cfg.ResolveAccountProfile("585912155334"); got != "dev-sso" {
		t.Errorf("got %q, want %q", got, "dev-sso")
	}
	if got := cfg.ResolveAccountProfile("999999999999"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveClusterContext(t *testing.T) {
	cfg := &Config{
		Contexts: map[string]string{
			"eks-dev": "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
		},
	}

	if got := cfg.ResolveClusterContext("eks-dev"); got == "" {
		t.Error("expected context for eks-dev")
	}
	if got := cfg.ResolveClusterContext("unknown"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestTargetNames(t *testing.T) {
	cfg := &Config{
		Targets: map[string]TargetConfig{
			"prod":  {},
			"dev":   {},
			"local": {},
		},
	}

	names := cfg.TargetNames()
	if len(names) != 3 {
		t.Fatalf("expected 3, got %d", len(names))
	}
	// Should be sorted.
	if names[0] != "dev" || names[1] != "local" || names[2] != "prod" {
		t.Errorf("expected [dev local prod], got %v", names)
	}
}
