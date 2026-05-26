package cmd

import (
	"reflect"
	"testing"

	"github.com/example/kestrel/internal/kubeconfig"
)

func TestMergeContextSources(t *testing.T) {
	kestContexts := map[string]string{
		"eks-dev":  "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
		"eks-prod": "arn:aws:eks:us-east-1:444455556666:cluster/eks-prod",
	}
	kubeContexts := []kubeconfig.Context{
		// Overlaps with eks-dev — should mark InKubeCfg, keep "eks-dev" display name.
		{Name: "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev"},
		// Only in kubeconfig — should use ShortName for display.
		{Name: "arn:aws:eks:eu-west-1:777788889999:cluster/eks-eu"},
		{Name: "kind-local"},
	}

	got := mergeContextSources(kestContexts, kubeContexts)

	if len(got) != 4 {
		t.Fatalf("got %d entries, want 4", len(got))
	}

	byDisplay := map[string]ContextEntry{}
	for _, e := range got {
		byDisplay[e.DisplayName] = e
	}

	devEntry, ok := byDisplay["eks-dev"]
	if !ok {
		t.Fatal("missing eks-dev entry")
	}
	if !devEntry.InGlobalCfg || !devEntry.InKubeCfg {
		t.Errorf("eks-dev should be in both sources, got InGlobalCfg=%v InKubeCfg=%v", devEntry.InGlobalCfg, devEntry.InKubeCfg)
	}

	prodEntry := byDisplay["eks-prod"]
	if !prodEntry.InGlobalCfg || prodEntry.InKubeCfg {
		t.Errorf("eks-prod should be global-only, got InGlobalCfg=%v InKubeCfg=%v", prodEntry.InGlobalCfg, prodEntry.InKubeCfg)
	}

	euEntry, ok := byDisplay["eks-eu"]
	if !ok {
		t.Fatal("missing eks-eu entry (kube-only, ShortName-derived)")
	}
	if euEntry.InGlobalCfg || !euEntry.InKubeCfg {
		t.Errorf("eks-eu should be kube-only, got InGlobalCfg=%v InKubeCfg=%v", euEntry.InGlobalCfg, euEntry.InKubeCfg)
	}
	if euEntry.Context != "arn:aws:eks:eu-west-1:777788889999:cluster/eks-eu" {
		t.Errorf("eks-eu context = %q", euEntry.Context)
	}

	if _, ok := byDisplay["kind-local"]; !ok {
		t.Fatal("missing kind-local entry")
	}

	// Should be sorted alphabetically by display name.
	wantOrder := []string{"eks-dev", "eks-eu", "eks-prod", "kind-local"}
	gotOrder := make([]string, len(got))
	for i, e := range got {
		gotOrder[i] = e.DisplayName
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Errorf("order = %v, want %v", gotOrder, wantOrder)
	}
}

func TestMergeContextSourcesEmpty(t *testing.T) {
	got := mergeContextSources(nil, nil)
	if len(got) != 0 {
		t.Errorf("empty inputs should produce empty result, got %d", len(got))
	}
}

func TestMergeContextSourcesKubeconfigOnly(t *testing.T) {
	got := mergeContextSources(nil, []kubeconfig.Context{
		{Name: "minikube"},
	})
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].DisplayName != "minikube" || got[0].Context != "minikube" {
		t.Errorf("entry = %+v", got[0])
	}
	if got[0].InGlobalCfg || !got[0].InKubeCfg {
		t.Errorf("source flags = InGlobalCfg=%v InKubeCfg=%v", got[0].InGlobalCfg, got[0].InKubeCfg)
	}
}

func TestMatchContexts(t *testing.T) {
	entries := []ContextEntry{
		{DisplayName: "eks-dev", Context: "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev"},
		{DisplayName: "eks-prod", Context: "arn:aws:eks:us-east-1:444455556666:cluster/eks-prod"},
		{DisplayName: "eks-staging", Context: "arn:aws:eks:us-east-1:777788889999:cluster/eks-staging"},
		{DisplayName: "kind-local", Context: "kind-local"},
	}

	tests := []struct {
		name  string
		query string
		want  []string // display names
	}{
		{"empty query", "", nil},
		{"exact display name", "eks-dev", []string{"eks-dev"}},
		{"exact full context", "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev", []string{"eks-dev"}},
		{"substring display name", "stag", []string{"eks-staging"}},
		{"substring matches multiple", "eks", []string{"eks-dev", "eks-prod", "eks-staging"}},
		{"case insensitive", "EKS-DEV", []string{"eks-dev"}},
		{"no match", "nope", nil},
		{"kind", "kind", []string{"kind-local"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchContexts(entries, tc.query)
			var names []string
			for _, e := range got {
				names = append(names, e.DisplayName)
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Errorf("matchContexts(%q) display names = %v, want %v", tc.query, names, tc.want)
			}
		})
	}
}

func TestMatchContextsShortNameOfARN(t *testing.T) {
	// When the entry's display name is itself an ARN (e.g. entry only in
	// kubeconfig with no friendly name), substring on ShortName should match.
	entries := []ContextEntry{
		{
			DisplayName: "eks-dev", // ShortName already; covers both paths
			Context:     "arn:aws:eks:us-east-1:111122223333:cluster/eks-dev",
		},
		{
			// Display falls back to context when no kest entry — simulate by
			// using the ARN as display too.
			DisplayName: "arn:aws:eks:us-east-1:444455556666:cluster/some-cluster",
			Context:     "arn:aws:eks:us-east-1:444455556666:cluster/some-cluster",
		},
	}
	got := matchContexts(entries, "some-cluster")
	if len(got) != 1 || got[0].DisplayName != "arn:aws:eks:us-east-1:444455556666:cluster/some-cluster" {
		t.Errorf("expected ShortName-based match, got %+v", got)
	}
}
