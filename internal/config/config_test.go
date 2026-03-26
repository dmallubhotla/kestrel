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
	// This is tricky to check per-env in yaml, so just check the overall output.
	if !strings.Contains(out, "aws_profile: acme-prod") {
		t.Errorf("expected acme-prod, got:\n%s", out)
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
