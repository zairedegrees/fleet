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
		Wake:       func(project, agent string) error { woke = append(woke, agent); return nil },
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
