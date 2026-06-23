package config

import "testing"

func TestNormalizeMapsAutoTalk(t *testing.T) {
	cfg := &FleetConfig{Agents: []AgentConfig{
		{Name: "a", AutoTalk: true},
		{Name: "b", AutoTalk: false},
		{Name: "c", Posture: PostureBounded, AutoTalk: true}, // explicit wins
	}}
	cfg.Normalize()
	if cfg.Agents[0].Posture != PostureAlways {
		t.Fatalf("auto_talk=true => always, got %q", cfg.Agents[0].Posture)
	}
	if cfg.Agents[1].Posture != PostureIdle {
		t.Fatalf("absent => idle, got %q", cfg.Agents[1].Posture)
	}
	if cfg.Agents[2].Posture != PostureBounded {
		t.Fatalf("explicit posture wins, got %q", cfg.Agents[2].Posture)
	}
	// AutoTalk is kept as a derived mirror of "greets at boot" (posture==always),
	// preserving the legacy field for older tooling/tests.
	if !cfg.Agents[0].AutoTalk {
		t.Fatal("always agent keeps AutoTalk=true mirror")
	}
	if cfg.Agents[1].AutoTalk || cfg.Agents[2].AutoTalk {
		t.Fatal("idle/bounded agents mirror AutoTalk=false")
	}
}

func TestEffectivePostureHelpers(t *testing.T) {
	if !(AgentConfig{Posture: PostureBounded}).IsBounded() {
		t.Fatal("IsBounded")
	}
	if !(AgentConfig{Posture: PostureAlways}).GreetsAtBoot() {
		t.Fatal("always greets at boot")
	}
	if (AgentConfig{Posture: PostureBounded}).GreetsAtBoot() {
		t.Fatal("bounded must NOT greet at boot")
	}
	if (AgentConfig{AutoTalk: true}).EffectivePosture() != PostureAlways {
		t.Fatal("pre-normalize auto_talk fallback")
	}
}
