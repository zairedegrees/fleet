package config

import "testing"

func TestResolveBoundedLayering(t *testing.T) {
	cfg := &FleetConfig{
		BoundedDefaults: &BoundedPolicy{Interval: "5m", BudgetUSD: 2},
		Agents:          []AgentConfig{{Name: "a", Posture: PostureBounded, Bounded: &BoundedPolicy{Interval: "1m"}}},
	}
	p := cfg.ResolveBounded(cfg.Agents[0])
	if p.Interval != "1m" {
		t.Fatalf("agent override wins: %q", p.Interval)
	}
	if p.BudgetUSD != 2 {
		t.Fatalf("fleet default fills gap: %v", p.BudgetUSD)
	}
	if p.MaxWakesPerDay != DefaultBoundedPolicy.MaxWakesPerDay {
		t.Fatalf("builtin default fills gap: %d", p.MaxWakesPerDay)
	}
	if p.CostPerWakeUSD != DefaultBoundedPolicy.CostPerWakeUSD {
		t.Fatalf("builtin cost default: %v", p.CostPerWakeUSD)
	}
}

func TestParseActiveHours(t *testing.T) {
	s, e, err := ParseActiveHours("09:00-19:00")
	if err != nil || s != 540 || e != 1140 {
		t.Fatalf("got %d,%d,%v", s, e, err)
	}
	if _, _, err := ParseActiveHours("19:00-09:00"); err == nil {
		t.Fatal("cross-midnight (start>=end) must error in v1")
	}
	if _, _, err := ParseActiveHours("9-19"); err == nil {
		t.Fatal("bad format must error")
	}
}

func TestBoundedValidate(t *testing.T) {
	if err := (BoundedPolicy{Interval: "nope"}).Validate(); err == nil {
		t.Fatal("bad interval")
	}
	if err := (BoundedPolicy{BudgetUSD: -1}).Validate(); err == nil {
		t.Fatal("negative budget")
	}
	if err := (BoundedPolicy{Interval: "10m", ActiveHours: "09:00-19:00"}).Validate(); err != nil {
		t.Fatalf("valid policy: %v", err)
	}
}

func TestConfigValidateRejectsBadPosture(t *testing.T) {
	cfg := &FleetConfig{Project: ProjectConfig{Name: "p"}, Agents: []AgentConfig{
		{Name: "a", Color: "green", Posture: "weird"},
	}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid posture must fail Validate")
	}
}
