package supervisor

import (
	"testing"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
)

func TestTickWakesDueAgentAndRecords(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var woke []string
	deps := Deps{
		Wake:       func(project, agent string) (bool, error) { woke = append(woke, agent); return true, nil },
		Productive: func(project, agent string) bool { return false }, // empty inbox
		Now:        func() time.Time { return time.Date(2026, 6, 23, 10, 0, 0, 0, time.Local) },
	}
	agents := []BoundedAgent{{Name: "a", Policy: config.BoundedPolicy{Interval: "10m", MaxWakesPerDay: 5, BudgetUSD: 1, CostPerWakeUSD: 0.06}}}
	if err := tick("demo", agents, deps); err != nil {
		t.Fatal(err)
	}
	if len(woke) != 1 || woke[0] != "a" {
		t.Fatalf("due agent woken: %v", woke)
	}
	st, _ := LoadState("demo")
	if st.Agents["a"].WakesToday != 1 || st.Agents["a"].EmptyStreak != 1 {
		t.Fatalf("tick must persist wake + empty outcome: %+v", st.Agents["a"])
	}
}

// A wake that reports it did NOT fire (busy/ghost pane) must be refunded: the
// provisional WakesToday/SpentUSD charge is rolled back, NextWakeAt is reset so
// the next tick retries, and RecordOutcome is not applied (no backoff streak).
func TestTickRefundsPhantomWake(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	deps := Deps{
		Wake:       func(project, agent string) (bool, error) { return false, nil },
		Productive: func(project, agent string) bool { t.Fatal("Productive must not run for a phantom wake"); return false },
		Now:        func() time.Time { return now },
	}
	agents := []BoundedAgent{{Name: "a", Policy: config.BoundedPolicy{Interval: "10m", MaxWakesPerDay: 5, BudgetUSD: 1, CostPerWakeUSD: 0.06}}}

	t.Setenv("HOME", t.TempDir())
	if err := tick("demo", agents, deps); err != nil {
		t.Fatalf("tick: %v", err)
	}
	st, err := LoadState("demo")
	if err != nil {
		t.Fatal(err)
	}
	as := st.Agents["a"]
	if as == nil {
		t.Fatal("agent state missing")
	}
	if as.WakesToday != 0 {
		t.Errorf("WakesToday = %d, want 0 (refunded)", as.WakesToday)
	}
	if as.SpentUSD != 0 {
		t.Errorf("SpentUSD = %v, want 0 (refunded)", as.SpentUSD)
	}
	if as.EmptyStreak != 0 {
		t.Errorf("EmptyStreak = %d, want 0 (RecordOutcome skipped)", as.EmptyStreak)
	}
	if !as.NextWakeAt.Equal(now) {
		t.Errorf("NextWakeAt = %v, want now=%v (retry next tick)", as.NextWakeAt, now)
	}
}
