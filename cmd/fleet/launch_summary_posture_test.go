package main

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

func TestLaunchSummaryCountsBounded(t *testing.T) {
	cfg := &config.FleetConfig{Agents: []config.AgentConfig{
		{Name: "a", Posture: config.PostureAlways},
		{Name: "b", Posture: config.PostureBounded},
		{Name: "c", Posture: config.PostureIdle},
	}}
	out := launchSummary(cfg)
	if !strings.Contains(out, "bounded") {
		t.Fatalf("summary must mention bounded: %q", out)
	}
	// The legacy always/on-demand line stays intact (1 always, 1 on-demand).
	if !strings.Contains(out, "1 greet at boot (auto-talk) · 1 on-demand") {
		t.Fatalf("legacy posture line must remain: %q", out)
	}
}
