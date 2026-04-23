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

aws:
  accounts:
    "585912155334":
      aws_profile: dev-sso
    "593671994769":
      aws_profile: prd-sso

kubernetes:
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

	if len(cfg.AWS.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(cfg.AWS.Accounts))
	}
	if cfg.AWS.Accounts["585912155334"].AwsProfile != "dev-sso" {
		t.Errorf("accounts[585912155334] = %q", cfg.AWS.Accounts["585912155334"].AwsProfile)
	}

	if cfg.Kubernetes.Contexts["eks-dev"] != "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev" {
		t.Errorf("contexts[eks-dev] = %q", cfg.Kubernetes.Contexts["eks-dev"])
	}

	if cfg.Directories["prd"] != "593671994769" {
		t.Errorf("directories[prd] = %q", cfg.Directories["prd"])
	}
}

func TestLoadFile_TargetWithAccountAndRegion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte(`
targets:
  dev:
    cluster: eks-dev
    aws_account: "585912155334"
    region: us-east-1
  prod:
    cluster: eks-prod
    aws_account: "593671994769"
    region: us-east-1
  local:
    cluster: kind-local
`), 0o644)

	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	dev := cfg.Targets["dev"]
	if dev.Cluster != "eks-dev" {
		t.Errorf("dev.Cluster = %q", dev.Cluster)
	}
	if dev.AWSAccount != "585912155334" {
		t.Errorf("dev.AWSAccount = %q", dev.AWSAccount)
	}
	if dev.Region != "us-east-1" {
		t.Errorf("dev.Region = %q", dev.Region)
	}

	local := cfg.Targets["local"]
	if local.AWSAccount != "" {
		t.Errorf("local.AWSAccount should be empty, got %q", local.AWSAccount)
	}
	if local.Region != "" {
		t.Errorf("local.Region should be empty, got %q", local.Region)
	}
}

func TestLoadFile_SwoopConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte(`
swoop:
  cd_mode: pushd
  editor: nvim
  sort_order: alpha
terraform:
  auto_install_tfenv: true
  write_version: true
  default_version: "1.9.2"
`), 0o644)

	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	if cfg.Swoop.CDMode != "pushd" {
		t.Errorf("Swoop.CDMode = %q, want %q", cfg.Swoop.CDMode, "pushd")
	}
	if cfg.Swoop.Editor != "nvim" {
		t.Errorf("Swoop.Editor = %q, want %q", cfg.Swoop.Editor, "nvim")
	}
	if cfg.Swoop.SortOrder != "alpha" {
		t.Errorf("Swoop.SortOrder = %q, want %q", cfg.Swoop.SortOrder, "alpha")
	}
	if !cfg.Terraform.AutoInstallTfenv {
		t.Error("Terraform.AutoInstallTfenv should be true")
	}
	if !cfg.Terraform.WriteVersion {
		t.Error("Terraform.WriteVersion should be true")
	}
	if cfg.Terraform.DefaultVersion != "1.9.2" {
		t.Errorf("Terraform.DefaultVersion = %q, want %q", cfg.Terraform.DefaultVersion, "1.9.2")
	}
}

func TestCompose_SwoopFromUser(t *testing.T) {
	user := &Config{
		Swoop: SwoopConfig{
			CDMode:    "pushd",
			Editor:    "nvim",
			SortOrder: "alpha",
		},
		Terraform: TerraformConfig{
			AutoInstallTfenv: true,
			WriteVersion:     true,
			DefaultVersion:   "1.9.2",
		},
	}
	project := &Config{
		Targets: map[string]TargetConfig{
			"dev": {Cluster: "eks-dev"},
		},
	}

	out := compose(user, project)

	if out.Swoop.CDMode != "pushd" {
		t.Errorf("Swoop.CDMode = %q, want %q", out.Swoop.CDMode, "pushd")
	}
	if out.Swoop.Editor != "nvim" {
		t.Errorf("Swoop.Editor = %q, want %q", out.Swoop.Editor, "nvim")
	}
	if out.Swoop.SortOrder != "alpha" {
		t.Errorf("Swoop.SortOrder = %q, want %q", out.Swoop.SortOrder, "alpha")
	}
	if !out.Terraform.AutoInstallTfenv {
		t.Error("Terraform.AutoInstallTfenv should be preserved from user config")
	}
	if !out.Terraform.WriteVersion {
		t.Error("Terraform.WriteVersion should be preserved from user config")
	}
	if out.Terraform.DefaultVersion != "1.9.2" {
		t.Errorf("Terraform.DefaultVersion = %q, want %q", out.Terraform.DefaultVersion, "1.9.2")
	}
}

func TestCompose_ProjectOverridesUser(t *testing.T) {
	user := &Config{
		Helm: HelmConfig{Chart: "user-chart", Namespace: "user-ns"},
		AWS: AWSConfig{
			Accounts: map[string]AWSAccountConfig{
				"585912155334": {AwsProfile: "dev-sso"},
			},
		},
		Kubernetes: KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev": "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
			},
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
	// AWS accounts from user preserved.
	if out.AWS.Accounts["585912155334"].AwsProfile != "dev-sso" {
		t.Errorf("AWS.Accounts[585912155334] = %v", out.AWS.Accounts["585912155334"])
	}
	// Kubernetes contexts from user preserved.
	if out.Kubernetes.Contexts["eks-dev"] == "" {
		t.Error("Kubernetes.Contexts[eks-dev] should be preserved from user")
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
		AWS: AWSConfig{
			Accounts: map[string]AWSAccountConfig{
				"585912155334": {AwsProfile: "dev-sso"},
			},
		},
		Kubernetes: KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev":    "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
				"kind-local": "kind-local",
			},
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
	if resolved.AccountID != "585912155334" {
		t.Errorf("dev.AccountID = %q, want 585912155334", resolved.AccountID)
	}
	if resolved.Cluster != "eks-dev" {
		t.Errorf("dev.Cluster = %q, want eks-dev", resolved.Cluster)
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

func TestResolveTarget_ExplicitAccount(t *testing.T) {
	cfg := &Config{
		Targets: map[string]TargetConfig{
			"dev": {
				Cluster:    "eks-dev",
				AWSAccount: "585912155334",
				Region:     "us-east-1",
			},
			"prod": {
				Cluster:    "eks-prod",
				AWSAccount: "593671994769",
				Region:     "us-east-1",
			},
		},
		AWS: AWSConfig{
			Accounts: map[string]AWSAccountConfig{
				"585912155334": {AwsProfile: "dev-sso"},
				"593671994769": {AwsProfile: "prd-sso"},
			},
		},
		Kubernetes: KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev":  "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
				"eks-prod": "arn:aws:eks:us-east-1:593671994769:cluster/eks-prod",
			},
		},
	}

	resolved, err := cfg.ResolveTarget("dev")
	if err != nil {
		t.Fatalf("ResolveTarget(dev): %v", err)
	}
	if resolved.AwsProfile != "dev-sso" {
		t.Errorf("dev.AwsProfile = %q, want dev-sso", resolved.AwsProfile)
	}
	if resolved.AccountID != "585912155334" {
		t.Errorf("dev.AccountID = %q, want 585912155334", resolved.AccountID)
	}
	if resolved.Region != "us-east-1" {
		t.Errorf("dev.Region = %q, want us-east-1", resolved.Region)
	}
	if resolved.Cluster != "eks-dev" {
		t.Errorf("dev.Cluster = %q, want eks-dev", resolved.Cluster)
	}

	resolved, err = cfg.ResolveTarget("prod")
	if err != nil {
		t.Fatalf("ResolveTarget(prod): %v", err)
	}
	if resolved.AwsProfile != "prd-sso" {
		t.Errorf("prod.AwsProfile = %q, want prd-sso", resolved.AwsProfile)
	}
	if resolved.AccountID != "593671994769" {
		t.Errorf("prod.AccountID = %q, want 593671994769", resolved.AccountID)
	}
}

func TestResolveTarget_AccountOnly(t *testing.T) {
	// Target with aws_account but no cluster (terraform-only target).
	cfg := &Config{
		Targets: map[string]TargetConfig{
			"prd": {
				AWSAccount: "593671994769",
				Region:     "us-east-1",
			},
		},
		AWS: AWSConfig{
			Accounts: map[string]AWSAccountConfig{
				"593671994769": {AwsProfile: "prd-sso"},
			},
		},
	}

	resolved, err := cfg.ResolveTarget("prd")
	if err != nil {
		t.Fatalf("ResolveTarget(prd): %v", err)
	}
	if resolved.KubeContext != "" {
		t.Errorf("prd.KubeContext should be empty, got %q", resolved.KubeContext)
	}
	if resolved.AwsProfile != "prd-sso" {
		t.Errorf("prd.AwsProfile = %q, want prd-sso", resolved.AwsProfile)
	}
	if resolved.AccountID != "593671994769" {
		t.Errorf("prd.AccountID = %q, want 593671994769", resolved.AccountID)
	}
	if resolved.Region != "us-east-1" {
		t.Errorf("prd.Region = %q, want us-east-1", resolved.Region)
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
		AWS: AWSConfig{
			Accounts: map[string]AWSAccountConfig{
				"585912155334": {AwsProfile: "dev-sso"},
			},
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
		Kubernetes: KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev": "arn:aws:eks:us-east-1:585912155334:cluster/eks-dev",
			},
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
