package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMarshal_OmitsEmptyFields(t *testing.T) {
	// A config with only terraform.iac_dir and one environment should not
	// include empty helm fields or empty env fields.
	cfg := &Config{
		Terraform: TerraformConfig{
			IACDir: "misc/iac",
		},
		Environments: map[string]EnvConfig{
			"dev": {AwsProfile: "acme-dev"},
		},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	// Should NOT contain helm section at all.
	if strings.Contains(out, "helm:") {
		t.Errorf("expected no helm section, got:\n%s", out)
	}

	// Should NOT contain empty string fields.
	if strings.Contains(out, "chart:") {
		t.Errorf("expected no chart field, got:\n%s", out)
	}
	if strings.Contains(out, "kube_context:") {
		t.Errorf("expected no kube_context field, got:\n%s", out)
	}
	if strings.Contains(out, "aws_account_id:") {
		t.Errorf("expected no aws_account_id field, got:\n%s", out)
	}

	// Should contain the fields we set.
	if !strings.Contains(out, "iac_dir: misc/iac") {
		t.Errorf("expected iac_dir, got:\n%s", out)
	}
	if !strings.Contains(out, "aws_profile: acme-dev") {
		t.Errorf("expected aws_profile, got:\n%s", out)
	}
}

func TestMarshal_OmitsEmptyEnvironments(t *testing.T) {
	cfg := &Config{}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if strings.Contains(out, "environments:") {
		t.Errorf("expected no environments section, got:\n%s", out)
	}
	if strings.Contains(out, "helm:") {
		t.Errorf("expected no helm section, got:\n%s", out)
	}
	if strings.Contains(out, "terraform:") {
		t.Errorf("expected no terraform section, got:\n%s", out)
	}
}

func TestMarshal_FullConfig(t *testing.T) {
	cfg := &Config{
		Helm: HelmConfig{
			Chart:     "oci://ghcr.io/example/chart:1.0",
			ValuesDir: "misc/chart",
		},
		Terraform: TerraformConfig{
			IACDir: "misc/iac",
		},
		Environments: map[string]EnvConfig{
			"dev":  {AwsProfile: "acme-dev", KubeContext: "dev-cluster"},
			"prod": {AwsProfile: "acme-prod"},
		},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	// Helm fields that are set should be present.
	if !strings.Contains(out, "chart: oci://ghcr.io/example/chart:1.0") {
		t.Errorf("expected chart, got:\n%s", out)
	}
	// Helm fields that are empty should NOT be present.
	if strings.Contains(out, "release_name:") {
		t.Errorf("expected no release_name, got:\n%s", out)
	}
	if strings.Contains(out, "namespace:") {
		t.Errorf("expected no namespace, got:\n%s", out)
	}
	if strings.Contains(out, "deploy_scripts:") {
		t.Errorf("expected no deploy_scripts, got:\n%s", out)
	}

	// Prod env should have aws_profile but no kube_context.
	if !strings.Contains(out, "aws_profile: acme-prod") {
		t.Errorf("expected acme-prod, got:\n%s", out)
	}
}

func TestMarshal_ProjectEnvFields(t *testing.T) {
	cfg := &Config{
		Environments: map[string]EnvConfig{
			"prod": {
				AwsAccountID: "222222222222",
				Region:       "us-east-1",
				Cluster:      "eks-prd",
			},
		},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if !strings.Contains(out, "aws_account_id: \"222222222222\"") {
		t.Errorf("expected aws_account_id, got:\n%s", out)
	}
	if !strings.Contains(out, "region: us-east-1") {
		t.Errorf("expected region, got:\n%s", out)
	}
	if !strings.Contains(out, "cluster: eks-prd") {
		t.Errorf("expected cluster, got:\n%s", out)
	}
	// Access fields should be absent.
	if strings.Contains(out, "aws_profile:") {
		t.Errorf("expected no aws_profile, got:\n%s", out)
	}
	if strings.Contains(out, "kube_context:") {
		t.Errorf("expected no kube_context, got:\n%s", out)
	}
}

func TestUnmarshal_RoundTrip(t *testing.T) {
	input := `terraform:
    iac_dir: misc/iac
environments:
    dev:
        aws_profile: acme-dev
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Terraform.IACDir != "misc/iac" {
		t.Errorf("iac_dir = %q", cfg.Terraform.IACDir)
	}
	if cfg.Environments["dev"].AwsProfile != "acme-dev" {
		t.Errorf("aws_profile = %q", cfg.Environments["dev"].AwsProfile)
	}
	// Helm should be zero-value.
	if cfg.Helm.Chart != "" {
		t.Errorf("expected empty chart, got %q", cfg.Helm.Chart)
	}
}

func TestUnmarshal_ProjectFields(t *testing.T) {
	input := `environments:
    prod:
        aws_account_id: "222222222222"
        region: us-east-1
        cluster: eks-prd
    local:
        cluster: kind-local
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatal(err)
	}

	prod := cfg.Environments["prod"]
	if prod.AwsAccountID != "222222222222" {
		t.Errorf("aws_account_id = %q", prod.AwsAccountID)
	}
	if prod.Region != "us-east-1" {
		t.Errorf("region = %q", prod.Region)
	}
	if prod.Cluster != "eks-prd" {
		t.Errorf("cluster = %q", prod.Cluster)
	}

	local := cfg.Environments["local"]
	if local.Cluster != "kind-local" {
		t.Errorf("local cluster = %q", local.Cluster)
	}
	if local.AwsAccountID != "" {
		t.Errorf("expected empty aws_account_id for local, got %q", local.AwsAccountID)
	}
}

// --- compose tests ---

func TestComposeEnvs_ProjectAuthoritative(t *testing.T) {
	user := map[string]EnvConfig{
		"dev":     {AwsProfile: "acme-dev", KubeContext: "dev-ctx"},
		"sandbox": {AwsProfile: "my-sandbox"}, // user-only, should be excluded
	}
	project := map[string]EnvConfig{
		"dev":  {AwsAccountID: "111111111111", Region: "us-east-1", Cluster: "eks-dev"},
		"prod": {AwsAccountID: "222222222222", Region: "us-east-1", Cluster: "eks-prd"},
	}

	envs := composeEnvs(user, project)

	// Only project-defined envs should be present.
	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d: %v", len(envs), envs)
	}
	if _, ok := envs["sandbox"]; ok {
		t.Error("sandbox should not be in resolved envs")
	}

	// dev should have both layers.
	dev := envs["dev"]
	if dev.AwsAccountID != "111111111111" {
		t.Errorf("dev.AwsAccountID = %q", dev.AwsAccountID)
	}
	if dev.AwsProfile != "acme-dev" {
		t.Errorf("dev.AwsProfile = %q", dev.AwsProfile)
	}
	if dev.KubeContext != "dev-ctx" {
		t.Errorf("dev.KubeContext = %q", dev.KubeContext)
	}
	if dev.Region != "us-east-1" {
		t.Errorf("dev.Region = %q", dev.Region)
	}

	// prod should have project fields only (user hasn't configured access).
	prod := envs["prod"]
	if prod.AwsAccountID != "222222222222" {
		t.Errorf("prod.AwsAccountID = %q", prod.AwsAccountID)
	}
	if prod.AwsProfile != "" {
		t.Errorf("prod.AwsProfile should be empty, got %q", prod.AwsProfile)
	}
}

func TestComposeEnvs_NoProject_FallsBackToUser(t *testing.T) {
	user := map[string]EnvConfig{
		"dev":  {AwsProfile: "acme-dev", KubeContext: "dev-ctx"},
		"prod": {AwsProfile: "acme-prod"},
	}
	var project map[string]EnvConfig // nil — no project config

	envs := composeEnvs(user, project)

	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d", len(envs))
	}
	if envs["dev"].AwsProfile != "acme-dev" {
		t.Errorf("dev.AwsProfile = %q", envs["dev"].AwsProfile)
	}
	if envs["prod"].AwsProfile != "acme-prod" {
		t.Errorf("prod.AwsProfile = %q", envs["prod"].AwsProfile)
	}
}

func TestComposeEnvs_EmptyProjectEnvs_FallsBackToUser(t *testing.T) {
	user := map[string]EnvConfig{
		"dev": {AwsProfile: "acme-dev"},
	}
	project := map[string]EnvConfig{} // empty map — no envs defined

	envs := composeEnvs(user, project)

	if len(envs) != 1 {
		t.Fatalf("expected 1 env, got %d", len(envs))
	}
	if envs["dev"].AwsProfile != "acme-dev" {
		t.Errorf("dev.AwsProfile = %q", envs["dev"].AwsProfile)
	}
}

func TestComposeEnvs_LocalClusterNoAWS(t *testing.T) {
	user := map[string]EnvConfig{
		"local": {KubeContext: "kind-local"},
	}
	project := map[string]EnvConfig{
		"local": {Cluster: "kind-local"},
	}

	envs := composeEnvs(user, project)

	local := envs["local"]
	if local.Cluster != "kind-local" {
		t.Errorf("local.Cluster = %q", local.Cluster)
	}
	if local.KubeContext != "kind-local" {
		t.Errorf("local.KubeContext = %q", local.KubeContext)
	}
	if local.AwsAccountID != "" {
		t.Errorf("local.AwsAccountID should be empty, got %q", local.AwsAccountID)
	}
	if local.AwsProfile != "" {
		t.Errorf("local.AwsProfile should be empty, got %q", local.AwsProfile)
	}
}

func TestComposeEnvs_SameAccountMultipleClusters(t *testing.T) {
	user := map[string]EnvConfig{
		"stage": {AwsProfile: "acme-prod-sso", KubeContext: "eks-stage-ctx"},
		"prod":  {AwsProfile: "acme-prod-sso", KubeContext: "eks-prd-ctx"},
		"ci":    {AwsProfile: "acme-prod-sso", KubeContext: "eks-ci-ctx"},
	}
	project := map[string]EnvConfig{
		"stage": {AwsAccountID: "222222222222", Region: "us-east-1", Cluster: "eks-stage"},
		"prod":  {AwsAccountID: "222222222222", Region: "us-east-1", Cluster: "eks-prd"},
		"ci":    {AwsAccountID: "222222222222", Region: "us-east-1", Cluster: "eks-ci"},
	}

	envs := composeEnvs(user, project)

	if len(envs) != 3 {
		t.Fatalf("expected 3 envs, got %d", len(envs))
	}

	// All share the same account and profile but have different clusters/contexts.
	for _, name := range []string{"stage", "prod", "ci"} {
		env := envs[name]
		if env.AwsAccountID != "222222222222" {
			t.Errorf("%s.AwsAccountID = %q", name, env.AwsAccountID)
		}
		if env.AwsProfile != "acme-prod-sso" {
			t.Errorf("%s.AwsProfile = %q", name, env.AwsProfile)
		}
	}

	if envs["stage"].Cluster != "eks-stage" {
		t.Errorf("stage.Cluster = %q", envs["stage"].Cluster)
	}
	if envs["prod"].Cluster != "eks-prd" {
		t.Errorf("prod.Cluster = %q", envs["prod"].Cluster)
	}
	if envs["ci"].Cluster != "eks-ci" {
		t.Errorf("ci.Cluster = %q", envs["ci"].Cluster)
	}
}

func TestCompose_PreservesRawLayers(t *testing.T) {
	user := &Config{
		Environments: map[string]EnvConfig{
			"dev": {AwsProfile: "acme-dev"},
		},
	}
	project := &Config{
		Terraform: TerraformConfig{IACDir: "misc/iac"},
		Environments: map[string]EnvConfig{
			"dev": {AwsAccountID: "111111111111", Region: "us-east-1"},
		},
	}

	out := compose(user, project)

	// Raw layers should be preserved.
	if len(out.ProjectEnvs) != 1 {
		t.Fatalf("expected 1 project env, got %d", len(out.ProjectEnvs))
	}
	if out.ProjectEnvs["dev"].AwsAccountID != "111111111111" {
		t.Errorf("ProjectEnvs[dev].AwsAccountID = %q", out.ProjectEnvs["dev"].AwsAccountID)
	}

	if len(out.UserEnvs) != 1 {
		t.Fatalf("expected 1 user env, got %d", len(out.UserEnvs))
	}
	if out.UserEnvs["dev"].AwsProfile != "acme-dev" {
		t.Errorf("UserEnvs[dev].AwsProfile = %q", out.UserEnvs["dev"].AwsProfile)
	}

	// Resolved should have both.
	if out.Environments["dev"].AwsAccountID != "111111111111" {
		t.Errorf("resolved dev.AwsAccountID = %q", out.Environments["dev"].AwsAccountID)
	}
	if out.Environments["dev"].AwsProfile != "acme-dev" {
		t.Errorf("resolved dev.AwsProfile = %q", out.Environments["dev"].AwsProfile)
	}
}

func TestCompose_HelmTerraformMerge(t *testing.T) {
	user := &Config{
		Helm: HelmConfig{
			Namespace: "default-ns",
		},
	}
	project := &Config{
		Helm: HelmConfig{
			Chart:     "oci://ghcr.io/org/chart:1.0",
			ValuesDir: "misc/chart",
		},
		Terraform: TerraformConfig{
			IACDir: "misc/iac",
		},
	}

	out := compose(user, project)

	// Project fields should override.
	if out.Helm.Chart != "oci://ghcr.io/org/chart:1.0" {
		t.Errorf("Chart = %q", out.Helm.Chart)
	}
	if out.Terraform.IACDir != "misc/iac" {
		t.Errorf("IACDir = %q", out.Terraform.IACDir)
	}
	// User field that project doesn't set should be preserved.
	if out.Helm.Namespace != "default-ns" {
		t.Errorf("Namespace = %q", out.Helm.Namespace)
	}
}

func TestHasProjectEnvs(t *testing.T) {
	withProject := &Config{
		ProjectEnvs: map[string]EnvConfig{
			"dev": {AwsAccountID: "111111111111"},
		},
	}
	if !withProject.HasProjectEnvs() {
		t.Error("expected HasProjectEnvs=true")
	}

	withoutProject := &Config{}
	if withoutProject.HasProjectEnvs() {
		t.Error("expected HasProjectEnvs=false")
	}
}

func TestResolveEnv_MissingAccessConfig(t *testing.T) {
	cfg := &Config{
		Environments: map[string]EnvConfig{
			"prod":  {AwsAccountID: "222222222222", Region: "us-east-1", Cluster: "eks-prd"},
			"dev":   {AwsAccountID: "111111111111", AwsProfile: "acme-dev"},
			"local": {Cluster: "kind-local"}, // no aws_account_id, no profile needed
		},
	}

	// prod has aws_account_id but no aws_profile — should error.
	_, err := cfg.ResolveEnv("prod")
	if err == nil {
		t.Fatal("expected error for missing aws_profile")
	}
	if !strings.Contains(err.Error(), "aws_profile") {
		t.Errorf("error should mention aws_profile, got: %v", err)
	}
	if !strings.Contains(err.Error(), "autoconfigure") {
		t.Errorf("error should mention autoconfigure, got: %v", err)
	}

	// dev has both — should succeed.
	env, err := cfg.ResolveEnv("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.AwsProfile != "acme-dev" {
		t.Errorf("AwsProfile = %q", env.AwsProfile)
	}

	// local has no aws_account_id — no profile needed, should succeed.
	env, err = cfg.ResolveEnv("local")
	if err != nil {
		t.Fatalf("unexpected error for local: %v", err)
	}
	if env.Cluster != "kind-local" {
		t.Errorf("Cluster = %q", env.Cluster)
	}

	// nonexistent env — should error.
	_, err = cfg.ResolveEnv("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent env")
	}
}

func TestResolveEnv_AccountIDSharing(t *testing.T) {
	cfg := &Config{
		Environments: map[string]EnvConfig{
			"prd":        {AwsAccountID: "593671994769", AwsProfile: "prd-sso"},
			"ci":         {AwsAccountID: "593671994769"}, // same account, no direct profile
			"example-prod": {AwsAccountID: "593671994769"}, // same account, no direct profile
			"dev":        {AwsAccountID: "585912155334"}, // different account, no profile anywhere
		},
	}

	// ci shares account with prd which has a profile — should succeed.
	_, err := cfg.ResolveEnv("ci")
	if err != nil {
		t.Fatalf("unexpected error for ci (shared account): %v", err)
	}

	// example-prod same — should succeed.
	_, err = cfg.ResolveEnv("example-prod")
	if err != nil {
		t.Fatalf("unexpected error for example-prod (shared account): %v", err)
	}

	// dev has account ID but no profile anywhere for that account — should error.
	_, err = cfg.ResolveEnv("dev")
	if err == nil {
		t.Fatal("expected error for dev (no profile for its account)")
	}
}

func TestMergeEnvField(t *testing.T) {
	base := EnvConfig{
		AwsAccountID: "111111111111",
		KubeContext:  "old-ctx",
	}
	overlay := EnvConfig{
		KubeContext: "new-ctx",
		AwsProfile:  "my-profile",
	}

	merged := MergeEnvField(base, overlay)

	if merged.AwsAccountID != "111111111111" {
		t.Errorf("AwsAccountID = %q", merged.AwsAccountID)
	}
	if merged.KubeContext != "new-ctx" {
		t.Errorf("KubeContext = %q", merged.KubeContext)
	}
	if merged.AwsProfile != "my-profile" {
		t.Errorf("AwsProfile = %q", merged.AwsProfile)
	}
}
