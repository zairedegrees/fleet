package main

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

func TestLaunchSummaryCountsPosture(t *testing.T) {
	cfg := &config.FleetConfig{
		Agents: []config.AgentConfig{
			{Name: "dev", AutoTalk: true},
			{Name: "auditor", AutoTalk: false},
			{Name: "ops", AutoTalk: false},
		},
	}
	out := launchSummary(cfg)
	if !strings.Contains(out, "1 greet at boot (auto-talk) · 2 on-demand") {
		t.Errorf("expected posture counts, got: %q", out)
	}
	if !strings.Contains(out, "fleet dispatch --to <agent>") {
		t.Errorf("expected wake hint, got: %q", out)
	}
	if !strings.Contains(out, "fleet --status") {
		t.Errorf("expected status hint, got: %q", out)
	}
}

func TestLaunchSummaryAllOnDemand(t *testing.T) {
	cfg := &config.FleetConfig{
		Agents: []config.AgentConfig{{Name: "a"}, {Name: "b"}},
	}
	out := launchSummary(cfg)
	if !strings.Contains(out, "0 greet at boot (auto-talk) · 2 on-demand") {
		t.Errorf("expected 0 auto-talk, got: %q", out)
	}
}
