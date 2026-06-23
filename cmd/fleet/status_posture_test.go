package main

import (
	"strings"
	"testing"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/supervisor"
)

func TestAgentLinePostureLabel(t *testing.T) {
	line := agentLine(agentStatus{Agent: "a", RelayState: "active", Posture: config.PostureBounded, Tasks: 0}, time.Now())
	if !strings.Contains(line, "bounded") {
		t.Fatalf("bounded posture must show in line: %q", line)
	}
}

func TestAgentLinePostureFallsBackToAutoTalk(t *testing.T) {
	// Legacy callers set only AutoTalk; the label must be unchanged.
	on := agentLine(agentStatus{Agent: "a", RelayState: "active", AutoTalk: true}, time.Now())
	if !strings.Contains(on, "auto-talk") {
		t.Fatalf("AutoTalk=true must still read auto-talk: %q", on)
	}
	off := agentLine(agentStatus{Agent: "a", RelayState: "active", AutoTalk: false}, time.Now())
	if !strings.Contains(off, "on-demand") {
		t.Fatalf("AutoTalk=false must still read on-demand: %q", off)
	}
}

func TestBoundedBudgetSeg(t *testing.T) {
	seg := boundedBudgetSeg(&supervisor.AgentState{WakesToday: 3, SpentUSD: 0.18}, config.BoundedPolicy{MaxWakesPerDay: 50, BudgetUSD: 3})
	if !strings.Contains(seg, "3/50") || !strings.Contains(seg, "$0.18") || !strings.Contains(seg, "$3.00") {
		t.Fatalf("budget seg: %q", seg)
	}
}

func TestAgentLineShowsBudgetWhenBounded(t *testing.T) {
	a := agentStatus{
		Agent: "a", RelayState: "active", Posture: config.PostureBounded, Tasks: 0,
		bounded:       &supervisor.AgentState{WakesToday: 2, SpentUSD: 0.12},
		boundedPolicy: config.BoundedPolicy{MaxWakesPerDay: 50, BudgetUSD: 3},
	}
	line := agentLine(a, time.Now())
	if !strings.Contains(line, "wakes 2/50") {
		t.Fatalf("bounded agent line must carry budget: %q", line)
	}
}
