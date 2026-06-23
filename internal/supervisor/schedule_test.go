package supervisor

import (
	"testing"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
)

func at(h, m int) time.Time { return time.Date(2026, 6, 23, h, m, 0, 0, time.Local) }

func pol() config.BoundedPolicy {
	return config.BoundedPolicy{Interval: "10m", MaxWakesPerDay: 3, BudgetUSD: 1, CostPerWakeUSD: 0.30}
}

func TestFirstTickWakes(t *testing.T) {
	st := &State{Agents: map[string]*AgentState{}}
	wake := decideWakes(st, []BoundedAgent{{Name: "a", Policy: pol()}}, at(10, 0))
	if len(wake) != 1 || wake[0] != "a" {
		t.Fatalf("first tick must wake new agent: %v", wake)
	}
	if st.Agents["a"].WakesToday != 1 {
		t.Fatalf("wakes counted: %d", st.Agents["a"].WakesToday)
	}
}

func TestNotYetDue(t *testing.T) {
	st := &State{Agents: map[string]*AgentState{}}
	decideWakes(st, []BoundedAgent{{Name: "a", Policy: pol()}}, at(10, 0))         // wakes, next=10:10
	wake := decideWakes(st, []BoundedAgent{{Name: "a", Policy: pol()}}, at(10, 5)) // before next
	if len(wake) != 0 {
		t.Fatalf("must not wake before next_wake_at: %v", wake)
	}
}

func TestBudgetCeiling(t *testing.T) {
	st := &State{Date: "2026-06-23", Agents: map[string]*AgentState{"a": {SpentUSD: 1.0, NextWakeAt: at(9, 0)}}}
	wake := decideWakes(st, []BoundedAgent{{Name: "a", Policy: pol()}}, at(10, 0))
	if len(wake) != 0 {
		t.Fatalf("budget exhausted must skip: %v", wake)
	}
}

func TestMaxWakesCeiling(t *testing.T) {
	st := &State{Date: "2026-06-23", Agents: map[string]*AgentState{"a": {WakesToday: 3, NextWakeAt: at(9, 0)}}}
	wake := decideWakes(st, []BoundedAgent{{Name: "a", Policy: pol()}}, at(10, 0))
	if len(wake) != 0 {
		t.Fatalf("max_wakes exhausted must skip: %v", wake)
	}
}

func TestOutsideActiveHours(t *testing.T) {
	p := pol()
	p.ActiveHours = "09:00-19:00"
	st := &State{Agents: map[string]*AgentState{}}
	wake := decideWakes(st, []BoundedAgent{{Name: "a", Policy: p}}, at(20, 0))
	if len(wake) != 0 {
		t.Fatalf("outside hours must skip: %v", wake)
	}
}

func TestBackoffDoublesAfterEmpty(t *testing.T) {
	p := pol()
	st := &State{Agents: map[string]*AgentState{"a": {IntervalSec: 600}}}
	RecordOutcome(st, "a", p, false, at(10, 0)) // empty #1 -> streak 1, no doubling yet
	if st.Agents["a"].IntervalSec != 600 {
		t.Fatalf("before threshold no doubling: %d", st.Agents["a"].IntervalSec)
	}
	RecordOutcome(st, "a", p, false, at(10, 0)) // empty #2 -> threshold -> double
	if st.Agents["a"].IntervalSec != 1200 {
		t.Fatalf("doubled at threshold: %d", st.Agents["a"].IntervalSec)
	}
}

func TestBackoffCapAndProductiveReset(t *testing.T) {
	p := pol()
	st := &State{Agents: map[string]*AgentState{"a": {IntervalSec: 3000, EmptyStreak: 5}}}
	RecordOutcome(st, "a", p, false, at(10, 0))
	if st.Agents["a"].IntervalSec != maxIntervalSec {
		t.Fatalf("capped at %d: got %d", maxIntervalSec, st.Agents["a"].IntervalSec)
	}
	RecordOutcome(st, "a", p, true, at(10, 0)) // productive resets to base (600s) + streak 0
	if st.Agents["a"].IntervalSec != 600 || st.Agents["a"].EmptyStreak != 0 {
		t.Fatalf("productive reset: %d / %d", st.Agents["a"].IntervalSec, st.Agents["a"].EmptyStreak)
	}
}

func TestDailyReset(t *testing.T) {
	st := &State{Date: "2026-06-22", Agents: map[string]*AgentState{"a": {WakesToday: 9, SpentUSD: 9, NextWakeAt: at(8, 0)}}}
	wake := decideWakes(st, []BoundedAgent{{Name: "a", Policy: pol()}}, at(10, 0))
	if st.Agents["a"].WakesToday != 1 || st.Date != "2026-06-23" {
		t.Fatalf("new day resets counters: %+v date=%s", st.Agents["a"], st.Date)
	}
	if len(wake) != 1 {
		t.Fatalf("after reset it wakes: %v", wake)
	}
}
