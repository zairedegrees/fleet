package main

import (
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

func TestSpawnSupervisorNoBoundedIsNoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "demo"},
		Agents:  []config.AgentConfig{{Name: "a", Posture: config.PostureIdle}},
	}
	if err := spawnSupervisor(cfg); err != nil {
		t.Fatalf("no bounded agents must be a clean no-op: %v", err)
	}
	if alive(supervisorPID("demo")) {
		t.Fatal("no supervisor should have been spawned")
	}
}

func TestBoundedCount(t *testing.T) {
	cfg := &config.FleetConfig{Agents: []config.AgentConfig{
		{Name: "a", Posture: config.PostureBounded},
		{Name: "b", Posture: config.PostureIdle},
		{Name: "c", Posture: config.PostureBounded},
	}}
	if n := boundedCount(cfg); n != 2 {
		t.Fatalf("boundedCount = %d, want 2", n)
	}
}
