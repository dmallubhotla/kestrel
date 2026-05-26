package kubeconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseContexts(t *testing.T) {
	data := []byte(`
apiVersion: v1
kind: Config
contexts:
- name: arn:aws:eks:us-east-1:111122223333:cluster/acme-dev
  context:
    cluster: arn:aws:eks:us-east-1:111122223333:cluster/acme-dev
    namespace: app
- name: acme-prod
  context:
    cluster: acme-prod-cluster
- name: minikube
  context:
    cluster: minikube
    namespace: default
`)

	contexts, err := ParseContexts(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(contexts) != 3 {
		t.Fatalf("got %d contexts, want 3", len(contexts))
	}

	if contexts[0].Name != "arn:aws:eks:us-east-1:111122223333:cluster/acme-dev" {
		t.Errorf("contexts[0].Name = %q", contexts[0].Name)
	}
	if contexts[0].Namespace != "app" {
		t.Errorf("contexts[0].Namespace = %q, want app", contexts[0].Namespace)
	}
}

func TestParseContextsEmpty(t *testing.T) {
	data := []byte(`
apiVersion: v1
kind: Config
contexts: []
`)
	contexts, err := ParseContexts(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 0 {
		t.Fatalf("got %d contexts, want 0", len(contexts))
	}
}

func TestReadContextsMergesMultipleFiles(t *testing.T) {
	dir := t.TempDir()

	fileA := filepath.Join(dir, "a.yaml")
	if err := os.WriteFile(fileA, []byte(`
apiVersion: v1
kind: Config
contexts:
- name: shared
  context:
    cluster: cluster-a
    namespace: from-a
- name: only-a
  context:
    cluster: cluster-a
`), 0o644); err != nil {
		t.Fatal(err)
	}

	fileB := filepath.Join(dir, "b.yaml")
	if err := os.WriteFile(fileB, []byte(`
apiVersion: v1
kind: Config
contexts:
- name: shared
  context:
    cluster: cluster-b
    namespace: from-b
- name: only-b
  context:
    cluster: cluster-b
`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KUBECONFIG", fileA+string(filepath.ListSeparator)+fileB)

	ctxs, err := ReadContexts()
	if err != nil {
		t.Fatal(err)
	}

	if len(ctxs) != 3 {
		t.Fatalf("got %d contexts, want 3 (shared+only-a+only-b)", len(ctxs))
	}

	byName := make(map[string]Context, len(ctxs))
	for _, c := range ctxs {
		byName[c.Name] = c
	}

	// First-wins: "shared" should come from fileA.
	if byName["shared"].Namespace != "from-a" {
		t.Errorf("shared.Namespace = %q, want from-a (first-wins)", byName["shared"].Namespace)
	}
	if _, ok := byName["only-a"]; !ok {
		t.Errorf("missing only-a context")
	}
	if _, ok := byName["only-b"]; !ok {
		t.Errorf("missing only-b context")
	}
}

func TestReadContextsSkipsEmptyEntries(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "kc.yaml")
	if err := os.WriteFile(file, []byte(`
apiVersion: v1
kind: Config
contexts:
- name: solo
  context:
    cluster: solo
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Leading/trailing/empty entries (e.g. "::file") must not cause errors.
	sep := string(filepath.ListSeparator)
	t.Setenv("KUBECONFIG", sep+file+sep)

	ctxs, err := ReadContexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(ctxs) != 1 || ctxs[0].Name != "solo" {
		t.Fatalf("got %+v, want [solo]", ctxs)
	}
}

func TestBestMatchAccountID(t *testing.T) {
	contexts := []Context{
		{Name: "arn:aws:eks:us-east-1:111122223333:cluster/acme-dev"},
		{Name: "arn:aws:eks:us-east-1:444455556666:cluster/acme-prod"},
		{Name: "minikube"},
	}

	// Account ID match.
	idx := BestMatch("acme-dev", "111122223333", contexts)
	if idx != 0 {
		t.Errorf("account ID match: got %d, want 0", idx)
	}

	// Account ID match for prod.
	idx = BestMatch("acme-prod", "444455556666", contexts)
	if idx != 1 {
		t.Errorf("account ID match prod: got %d, want 1", idx)
	}
}

func TestBestMatchNameSubstring(t *testing.T) {
	contexts := []Context{
		{Name: "acme-dev-cluster"},
		{Name: "acme-prod-cluster"},
		{Name: "minikube"},
	}

	idx := BestMatch("acme-dev", "", contexts)
	if idx != 0 {
		t.Errorf("substring match: got %d, want 0", idx)
	}

	idx = BestMatch("acme-prod", "", contexts)
	if idx != 1 {
		t.Errorf("substring match prod: got %d, want 1", idx)
	}
}

func TestBestMatchExactPreferred(t *testing.T) {
	contexts := []Context{
		{Name: "acme-dev-extra"},
		{Name: "acme-dev"},
	}

	idx := BestMatch("acme-dev", "", contexts)
	if idx != 1 {
		t.Errorf("exact match preferred: got %d, want 1", idx)
	}
}

func TestBestMatchNoMatch(t *testing.T) {
	contexts := []Context{
		{Name: "minikube"},
		{Name: "docker-desktop"},
	}

	idx := BestMatch("acme-dev", "", contexts)
	if idx != -1 {
		t.Errorf("no match: got %d, want -1", idx)
	}
}

func TestBestMatchAccountIDWithNameDisambiguation(t *testing.T) {
	// Two clusters in the same account — name should disambiguate.
	contexts := []Context{
		{Name: "arn:aws:eks:us-east-1:111122223333:cluster/acme-dev"},
		{Name: "arn:aws:eks:us-east-1:111122223333:cluster/acme-staging"},
	}

	idx := BestMatch("acme-dev", "111122223333", contexts)
	if idx != 0 {
		t.Errorf("disambiguated: got %d, want 0", idx)
	}

	idx = BestMatch("acme-staging", "111122223333", contexts)
	if idx != 1 {
		t.Errorf("disambiguated staging: got %d, want 1", idx)
	}
}

func TestShortName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"arn:aws:eks:us-east-1:111122223333:cluster/acme-dev", "acme-dev"},
		{"arn:aws:eks:eu-west-1:444455556666:cluster/prod-eu", "prod-eu"},
		{"acme-prod", "acme-prod"},
		{"minikube", "minikube"},
	}
	for _, tt := range tests {
		got := ShortName(tt.input)
		if got != tt.want {
			t.Errorf("ShortName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractAccountID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"arn:aws:eks:us-east-1:111122223333:cluster/acme-dev", "111122223333"},
		{"acme-prod", ""},
		{"minikube", ""},
	}
	for _, tt := range tests {
		got := ExtractAccountID(tt.input)
		if got != tt.want {
			t.Errorf("ExtractAccountID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
