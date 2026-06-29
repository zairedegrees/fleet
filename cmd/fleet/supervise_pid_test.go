package main

import (
	"os"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/supervisor"
)

// A corrupt state file makes LoadState return (nil, err); recording the PID must
// not panic and must leave a valid state carrying our PID.
func TestRecordSupervisorPIDSurvivesCorruptState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(config.FleetDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(supervisor.StatePath("demo"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := recordSupervisorPID("demo", 4242); err != nil {
		t.Fatalf("recordSupervisorPID on corrupt state: %v", err)
	}
	st, err := supervisor.LoadState("demo")
	if err != nil {
		t.Fatalf("state must be valid after record: %v", err)
	}
	if st.PID != 4242 {
		t.Errorf("PID = %d, want 4242", st.PID)
	}
}
