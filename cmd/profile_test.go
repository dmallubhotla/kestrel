package cmd

import (
	"reflect"
	"testing"

	"github.com/deepak-science/kestrel/internal/config"
)

func TestMatchTargets(t *testing.T) {
	cfg := &config.Config{
		Targets: map[string]config.TargetConfig{
			"dev":     {Cluster: "eks-dev"},
			"prod":    {Cluster: "eks-prod"},
			"staging": {Cluster: "eks-staging"},
			"local":   {Cluster: "kind-local"},
			"orphan":  {Cluster: "no-such-cluster"}, // ResolveTarget will error
		},
		Kubernetes: config.KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev":     "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
				"eks-prod":    "arn:aws:eks:us-east-1:444455556666:cluster/eks-prod",
				"eks-staging": "arn:aws:eks:us-east-1:777788889999:cluster/eks-staging",
				"kind-local":  "kind-local",
			},
		},
	}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"empty query", "", nil},
		{"exact target name", "dev", []string{"dev"}},
		{"substring of target name", "stag", []string{"staging"}},
		{"kube short name match (eks-dev)", "eks-dev", []string{"dev"}},
		{"kube short name substring", "prod", []string{"prod"}},
		{"kind context short name", "kind", []string{"local"}},
		{"no match", "nonexistent", nil},
		{"case-insensitive", "EKS-DEV", []string{"dev"}},
		{"orphan target skipped on context match", "no-such", nil},
		{"orphan still matchable by target name", "orphan", []string{"orphan"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchTargets(cfg, tc.query)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("matchTargets(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestMatchTargetsMultiple(t *testing.T) {
	cfg := &config.Config{
		Targets: map[string]config.TargetConfig{
			"dev-east": {Cluster: "eks-dev-east"},
			"dev-west": {Cluster: "eks-dev-west"},
			"prod":     {Cluster: "eks-prod"},
		},
		Kubernetes: config.KubernetesConfig{
			Contexts: map[string]string{
				"eks-dev-east": "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev-east",
				"eks-dev-west": "arn:aws:eks:us-west-2:111122223333:cluster/eks-dev-west",
				"eks-prod":     "arn:aws:eks:us-east-1:444455556666:cluster/eks-prod",
			},
		},
	}

	got := matchTargets(cfg, "dev")
	want := []string{"dev-east", "dev-west"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("matchTargets(dev) = %v, want %v", got, want)
	}
}
