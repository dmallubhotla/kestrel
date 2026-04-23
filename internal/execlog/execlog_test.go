package execlog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLogWritesJSONEntry(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "commands-*.log")
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	logFile = f
	encoder = json.NewEncoder(f)
	mu.Unlock()

	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	Log(Entry{
		Timestamp:  now,
		Command:    "terraform",
		Args:       []string{"plan", "-out=tfplan"},
		Dir:        "/tmp/roots/dev",
		ExitCode:   0,
		DurationMs: 4200,
	})

	Log(Entry{
		Timestamp:  now,
		Command:    "helm",
		Args:       []string{"upgrade", "--install"},
		ExitCode:   1,
		DurationMs: 300,
	})

	f.Close()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	dec := json.NewDecoder(strings.NewReader(string(data)))

	var first Entry
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode first entry: %v", err)
	}
	if first.Command != "terraform" {
		t.Errorf("command = %q, want terraform", first.Command)
	}
	if len(first.Args) != 2 || first.Args[1] != "-out=tfplan" {
		t.Errorf("args = %v, want [plan -out=tfplan]", first.Args)
	}
	if first.Dir != "/tmp/roots/dev" {
		t.Errorf("dir = %q, want /tmp/roots/dev", first.Dir)
	}
	if first.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", first.ExitCode)
	}
	if first.DurationMs != 4200 {
		t.Errorf("duration_ms = %d, want 4200", first.DurationMs)
	}

	var second Entry
	if err := dec.Decode(&second); err != nil {
		t.Fatalf("decode second entry: %v", err)
	}
	if second.Command != "helm" {
		t.Errorf("command = %q, want helm", second.Command)
	}
	if second.Dir != "" {
		t.Errorf("dir = %q, want empty (omitempty)", second.Dir)
	}
	if second.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", second.ExitCode)
	}
}

func TestLogNoopWithoutInit(t *testing.T) {
	mu.Lock()
	oldFile := logFile
	oldEnc := encoder
	logFile = nil
	encoder = nil
	mu.Unlock()

	defer func() {
		mu.Lock()
		logFile = oldFile
		encoder = oldEnc
		mu.Unlock()
	}()

	// Should not panic
	Log(Entry{Command: "echo", Args: []string{"hello"}})
}
