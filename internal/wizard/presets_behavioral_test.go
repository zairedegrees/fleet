package wizard

import (
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

// Every preset must produce a fleet that passes config.Validate() — this guards
// the authored personas/tools/models against an invalid char, model, or mode.
func TestAllPresetsValidate(t *testing.T) {
	for _, p := range AllPresets() {
		if len(p.Agents) == 0 {
			continue // Custom is intentionally empty
		}
		cfg := config.FleetConfig{
			Project: config.ProjectConfig{Name: "proj", Cwd: "/tmp"},
			Agents:  p.Agents,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("preset %q does not validate: %v", p.Name, err)
		}
	}
}

// The whole point of v0.1.3: every preset agent is behaviorally tuned — a model
// tier and an injected persona, not just a name+role.
func TestEveryPresetAgentIsBehaviorallyTuned(t *testing.T) {
	for _, p := range AllPresets() {
		for _, a := range p.Agents {
			if a.Model == "" {
				t.Errorf("preset %q agent %q has no model", p.Name, a.Name)
			}
			if a.Persona == "" {
				t.Errorf("preset %q agent %q has no persona", p.Name, a.Name)
			}
		}
	}
}

func TestNewBehavioralPresets(t *testing.T) {
	// The three flagship presets exist.
	for _, name := range []string{"Solo Pair", "Design Studio", "Security Hardening"} {
		if GetPreset(name) == nil {
			t.Fatalf("expected preset %q to exist", name)
		}
	}

	// Solo Pair: cheap hands (sonnet dev), expensive eyes (opus auditor in plan mode).
	solo := GetPreset("Solo Pair")
	dev := findAgent(t, solo.Agents, "dev")
	if dev.Model != "sonnet" {
		t.Errorf("Solo Pair dev model = %q, want sonnet", dev.Model)
	}
	auditor := findAgent(t, solo.Agents, "auditor")
	if auditor.Model != "opus" || auditor.PermissionMode != "plan" {
		t.Errorf("Solo Pair auditor = %q/%q, want opus/plan", auditor.Model, auditor.PermissionMode)
	}

	// Design Studio & Security Hardening are led by an executive architect.
	for _, name := range []string{"Design Studio", "Security Hardening"} {
		arch := findAgent(t, GetPreset(name).Agents, "architect")
		if !arch.IsExecutive {
			t.Errorf("%s architect must be executive", name)
		}
	}
}

func findAgent(t *testing.T, agents []config.AgentConfig, name string) config.AgentConfig {
	t.Helper()
	for _, a := range agents {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("agent %q not found", name)
	return config.AgentConfig{}
}
