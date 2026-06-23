package supervisor

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // config.FleetDir() = $HOME/.fleet
	st := &State{Project: "demo", Date: "2026-06-23", PID: 4242,
		Agents: map[string]*AgentState{"a": {WakesToday: 2, SpentUSD: 0.12, NextWakeAt: time.Now().Truncate(time.Second)}}}
	if err := SaveState(st); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.PID != 4242 || got.Agents["a"].WakesToday != 2 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if filepath.Base(StatePath("demo")) != "demo.supervisor.json" {
		t.Fatalf("path: %s", StatePath("demo"))
	}
}

func TestLoadMissingIsFresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := LoadState("nope")
	if err != nil {
		t.Fatalf("missing must not error: %v", err)
	}
	if got.Project != "nope" || got.Agents == nil {
		t.Fatalf("fresh state expected: %+v", got)
	}
}

func TestClearState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveState(&State{Project: "demo", Agents: map[string]*AgentState{}}); err != nil {
		t.Fatal(err)
	}
	if err := ClearState("demo"); err != nil {
		t.Fatal(err)
	}
	if err := ClearState("demo"); err != nil {
		t.Fatalf("clearing a missing state must be a no-op: %v", err)
	}
}
