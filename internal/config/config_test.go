package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(`
helm:
  chart: oci://ghcr.io/test/chart:1.0
  values_dir: misc/chart
  namespace: app
  releases:
    customer-25:
      release_name: my-app-customer-25
      target: dev
      values:
        - dev-customer-25.yaml
    other:
      release_name: my-app-other
      target: dev
      values:
        - dev-other.yaml

targets:
  dev:
    cluster: eks-dev
  prod:
    cluster: eks-prd
  local:
    cluster: kind-local

aws:
  accounts:
    "111122223333":
      aws_profile: dev-sso
    "444455556666":
      aws_profile: prd-sso

kubernetes:
  contexts:
    eks-dev: arn:aws:eks:us-east-1:111122223333:cluster/eks-dev
    eks-prd: arn:aws:eks:us-east-1:444455556666:cluster/eks-prd
    kind-local: kind-local

directories:
  prd: "444455556666"
  dev: "111122223333"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	if cfg.Helm.Chart != "oci://ghcr.io/test/chart:1.0" {
		t.Errorf("chart = %q", cfg.Helm.Chart)
	}

	if len(cfg.Helm.Releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(cfg.Helm.Releases))
	}
	r := cfg.Helm.Releases["customer-25"]
	if r.ReleaseName != "my-app-customer-25" {
		t.Errorf("releases[customer-25].release_name = %q", r.ReleaseName)
	}
	if r.Target != "dev" {
		t.Errorf("releases[customer-25].target = %q", r.Target)
	}
	if len(r.Values) != 1 || r.Values[0] != "dev-customer-25.yaml" {
		t.Errorf("releases[customer-25].values = %v", r.Values)
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
	if cfg.AWS.Accounts["111122223333"].AwsProfile != "dev-sso" {
		t.Errorf("accounts[111122223333] = %q", cfg.AWS.Accounts["111122223333"].AwsProfile)
	}

	if cfg.Kubernetes.Contexts["eks-dev"] != "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev" {
		t.Errorf("contexts[eks-dev] = %q", cfg.Kubernetes.Contexts["eks-dev"])
	}

	if cfg.Directories["prd"] != "444455556666" {
		t.Errorf("directories[prd] = %q", cfg.Directories["prd"])
	}
}

func TestLoadFile_TargetWithAccountAndRegion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(`
targets:
  dev:
    cluster: eks-dev
    aws_account: "111122223333"
    region: us-east-1
  prod:
    cluster: eks-prod
    aws_account: "444455556666"
    region: us-east-1
  local:
    cluster: kind-local
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	dev := cfg.Targets["dev"]
	if dev.Cluster != "eks-dev" {
		t.Errorf("dev.Cluster = %q", dev.Cluster)
	}
	if dev.AWSAccount != "111122223333" {
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
	if err := os.WriteFile(path, []byte(`
swoop:
  cd_mode: pushd
  editor: nvim
  sort_order: alpha
terraform:
  auto_install_tfenv: true
  write_version: true
  default_version: "1.9.2"
`), 0o644); err != nil {
		t.Fatal(err)
	}

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
				"111122223333": {AwsProfile: "dev-sso"},
			},
		},
		Kubernetes: KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev": "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
			},
		},
	}
	project := &Config{
		Helm: HelmConfig{
			Chart:     "project-chart",
			ValuesDir: "misc/chart",
			Releases: map[string]HelmRelease{
				"v1": {ReleaseName: "app-v1", Target: "dev"},
			},
		},
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
	// Releases from project.
	if len(out.Helm.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(out.Helm.Releases))
	}
	if out.Helm.Releases["v1"].ReleaseName != "app-v1" {
		t.Errorf("Helm.Releases[v1].ReleaseName = %q", out.Helm.Releases["v1"].ReleaseName)
	}
	// Targets from project.
	if len(out.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(out.Targets))
	}
	// AWS accounts from user preserved.
	if out.AWS.Accounts["111122223333"].AwsProfile != "dev-sso" {
		t.Errorf("AWS.Accounts[111122223333] = %v", out.AWS.Accounts["111122223333"])
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
				"111122223333": {AwsProfile: "dev-sso"},
			},
		},
		Kubernetes: KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev":    "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
				"kind-local": "kind-local",
			},
		},
	}

	// dev: has EKS context → extracts account → resolves profile.
	resolved, err := cfg.ResolveTarget("dev")
	if err != nil {
		t.Fatalf("ResolveTarget(dev): %v", err)
	}
	if resolved.KubeContext != "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev" {
		t.Errorf("dev.KubeContext = %q", resolved.KubeContext)
	}
	if resolved.AwsProfile != "dev-sso" {
		t.Errorf("dev.AwsProfile = %q", resolved.AwsProfile)
	}
	if resolved.AccountID != "111122223333" {
		t.Errorf("dev.AccountID = %q, want 111122223333", resolved.AccountID)
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
				AWSAccount: "111122223333",
				Region:     "us-east-1",
			},
			"prod": {
				Cluster:    "eks-prod",
				AWSAccount: "444455556666",
				Region:     "us-east-1",
			},
		},
		AWS: AWSConfig{
			Accounts: map[string]AWSAccountConfig{
				"111122223333": {AwsProfile: "dev-sso"},
				"444455556666": {AwsProfile: "prd-sso"},
			},
		},
		Kubernetes: KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev":  "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
				"eks-prod": "arn:aws:eks:us-east-1:444455556666:cluster/eks-prod",
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
	if resolved.AccountID != "111122223333" {
		t.Errorf("dev.AccountID = %q, want 111122223333", resolved.AccountID)
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
	if resolved.AccountID != "444455556666" {
		t.Errorf("prod.AccountID = %q, want 444455556666", resolved.AccountID)
	}
}

func TestResolveTarget_AccountOnly(t *testing.T) {
	// Target with aws_account but no cluster (terraform-only target).
	cfg := &Config{
		Targets: map[string]TargetConfig{
			"prd": {
				AWSAccount: "444455556666",
				Region:     "us-east-1",
			},
		},
		AWS: AWSConfig{
			Accounts: map[string]AWSAccountConfig{
				"444455556666": {AwsProfile: "prd-sso"},
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
	if resolved.AccountID != "444455556666" {
		t.Errorf("prd.AccountID = %q, want 444455556666", resolved.AccountID)
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
		{"arn:aws:eks:us-east-1:111122223333:cluster/eks-dev", "111122223333"},
		{"arn:aws:eks:us-west-2:444455556666:cluster/eks-prd", "444455556666"},
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
				"111122223333": {AwsProfile: "dev-sso"},
			},
		},
	}

	if got := cfg.ResolveAccountProfile("111122223333"); got != "dev-sso" {
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
				"eks-dev": "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
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

func TestReleaseNames(t *testing.T) {
	cfg := &Config{
		Helm: HelmConfig{
			Releases: map[string]HelmRelease{
				"other":       {ReleaseName: "app-other", Target: "dev"},
				"customer-25": {ReleaseName: "app-customer-25", Target: "dev"},
				"v1":          {ReleaseName: "app-v1", Target: "local"},
			},
		},
	}

	names := cfg.ReleaseNames()
	if len(names) != 3 {
		t.Fatalf("expected 3, got %d", len(names))
	}
	if names[0] != "customer-25" || names[1] != "other" || names[2] != "v1" {
		t.Errorf("expected [customer-25 other v1], got %v", names)
	}
}

func TestReleasesForTarget(t *testing.T) {
	cfg := &Config{
		Helm: HelmConfig{
			Releases: map[string]HelmRelease{
				"other":       {ReleaseName: "app-other", Target: "dev"},
				"customer-25": {ReleaseName: "app-customer-25", Target: "dev"},
				"v1":          {ReleaseName: "app-v1", Target: "local"},
			},
		},
	}

	devReleases := cfg.ReleasesForTarget("dev")
	if len(devReleases) != 2 {
		t.Fatalf("expected 2 dev releases, got %d", len(devReleases))
	}
	if devReleases[0] != "customer-25" || devReleases[1] != "other" {
		t.Errorf("expected [customer-25 other], got %v", devReleases)
	}

	localReleases := cfg.ReleasesForTarget("local")
	if len(localReleases) != 1 || localReleases[0] != "v1" {
		t.Errorf("expected [v1], got %v", localReleases)
	}

	prodReleases := cfg.ReleasesForTarget("prod")
	if len(prodReleases) != 0 {
		t.Errorf("expected 0 prod releases, got %d", len(prodReleases))
	}
}

func TestEffectiveDeployScripts(t *testing.T) {
	topLevel := []string{"migrate.sh"}
	perRelease := []string{"custom.sh"}
	empty := []string{}

	cfg := &Config{
		Helm: HelmConfig{
			DeployScripts: topLevel,
		},
	}

	// nil DeployScripts → inherit from HelmConfig
	r1 := HelmRelease{DeployScripts: nil}
	got := cfg.EffectiveDeployScripts(r1)
	if len(got) != 1 || got[0] != "migrate.sh" {
		t.Errorf("nil override: got %v, want %v", got, topLevel)
	}

	// explicit override
	r2 := HelmRelease{DeployScripts: &perRelease}
	got = cfg.EffectiveDeployScripts(r2)
	if len(got) != 1 || got[0] != "custom.sh" {
		t.Errorf("override: got %v, want %v", got, perRelease)
	}

	// explicit empty → skip scripts
	r3 := HelmRelease{DeployScripts: &empty}
	got = cfg.EffectiveDeployScripts(r3)
	if len(got) != 0 {
		t.Errorf("empty override: got %v, want []", got)
	}
}

func TestLoadFile_HelmReleases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(`
helm:
  chart: oci://ghcr.io/test/chart:1.0
  values_dir: chart
  namespace: app
  deploy_scripts:
    - migrate.sh
  releases:
    customer-25:
      release_name: owl-copy-customer-25
      target: dev
      values:
        - dev-customer-25.yaml
    other:
      release_name: owl-copy-other
      target: dev
      values:
        - dev-other.yaml
    v1:
      release_name: owl-copy-v1
      target: local
      values:
        - local.yaml
      deploy_scripts: []
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	if len(cfg.Helm.Releases) != 3 {
		t.Fatalf("expected 3 releases, got %d", len(cfg.Helm.Releases))
	}

	c25 := cfg.Helm.Releases["customer-25"]
	if c25.ReleaseName != "owl-copy-customer-25" {
		t.Errorf("customer-25.release_name = %q", c25.ReleaseName)
	}
	if c25.Target != "dev" {
		t.Errorf("customer-25.target = %q", c25.Target)
	}
	if len(c25.Values) != 1 || c25.Values[0] != "dev-customer-25.yaml" {
		t.Errorf("customer-25.values = %v", c25.Values)
	}
	if c25.DeployScripts != nil {
		t.Error("customer-25.deploy_scripts should be nil (inherit)")
	}

	v1 := cfg.Helm.Releases["v1"]
	if v1.DeployScripts == nil {
		t.Fatal("v1.deploy_scripts should not be nil (explicit empty)")
	}
	if len(*v1.DeployScripts) != 0 {
		t.Errorf("v1.deploy_scripts should be empty, got %v", *v1.DeployScripts)
	}

	// Top-level deploy scripts
	if len(cfg.Helm.DeployScripts) != 1 || cfg.Helm.DeployScripts[0] != "migrate.sh" {
		t.Errorf("helm.deploy_scripts = %v", cfg.Helm.DeployScripts)
	}
}
