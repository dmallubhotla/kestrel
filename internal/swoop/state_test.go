package swoop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adrg/xdg"
)

func TestState_RoundTrip(t *testing.T) {
	// Use a temp dir for XDG state.
	tmp := t.TempDir()
	origState := xdg.StateHome
	xdg.StateHome = tmp
	defer func() { xdg.StateHome = origState }()

	projectDir := filepath.Join(tmp, "myproject")
	os.MkdirAll(projectDir, 0o755)

	// Load (should be empty).
	state, err := LoadState(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 0 {
		t.Fatalf("expected empty state, got %d entries", len(state.Entries))
	}

	// Record some actions.
	state.RecordInit("dev/networking/vpc")
	state.RecordPlan("dev/networking/vpc", "0 to add, 1 to change, 0 to destroy")
	state.RecordApply("dev/data-stores/fhr-db")

	if err := state.Save(); err != nil {
		t.Fatal(err)
	}

	// Reload and verify.
	state2, err := LoadState(projectDir)
	if err != nil {
		t.Fatal(err)
	}

	vpc := state2.Entries["dev/networking/vpc"]
	if vpc == nil {
		t.Fatal("expected entry for dev/networking/vpc")
	}
	if vpc.LastInit == nil {
		t.Error("expected LastInit to be set")
	}
	if vpc.LastPlan == nil {
		t.Error("expected LastPlan to be set")
	}
	if vpc.PlanResult != "0 to add, 1 to change, 0 to destroy" {
		t.Errorf("PlanResult = %q", vpc.PlanResult)
	}

	fhr := state2.Entries["dev/data-stores/fhr-db"]
	if fhr == nil {
		t.Fatal("expected entry for dev/data-stores/fhr-db")
	}
	if fhr.LastApply == nil {
		t.Error("expected LastApply to be set")
	}
}

func TestState_RecordTimestamps(t *testing.T) {
	tmp := t.TempDir()
	origState := xdg.StateHome
	xdg.StateHome = tmp
	defer func() { xdg.StateHome = origState }()

	state, err := LoadState(tmp)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-time.Second)
	state.RecordPlan("test/root", "no changes")
	after := time.Now().UTC().Add(time.Second)

	e := state.Entries["test/root"]
	if e.LastPlan == nil {
		t.Fatal("LastPlan should be set")
	}
	if e.LastPlan.Before(before) || e.LastPlan.After(after) {
		t.Errorf("LastPlan %v not in expected range [%v, %v]", e.LastPlan, before, after)
	}
}
