package wizard

import "testing"

func TestAllPresets(t *testing.T) {
	presets := AllPresets()
	if len(presets) != 10 {
		t.Fatalf("expected 10 presets, got %d", len(presets))
	}

	// Verify each preset has a unique name
	seen := make(map[string]bool)
	for _, p := range presets {
		if seen[p.Name] {
			t.Errorf("duplicate preset name: %s", p.Name)
		}
		seen[p.Name] = true
		if p.Icon == "" {
			t.Errorf("preset %s has no icon", p.Name)
		}
	}

	// Custom should have 0 agents
	custom := GetPreset("Custom")
	if custom == nil {
		t.Fatal("Custom preset not found")
	}
	if len(custom.Agents) != 0 {
		t.Errorf("Custom should have 0 agents, got %d", len(custom.Agents))
	}

	// Web App should have 5 agents
	webapp := GetPreset("Web App")
	if webapp == nil {
		t.Fatal("Web App preset not found")
	}
	if len(webapp.Agents) != 5 {
		t.Errorf("Web App should have 5 agents, got %d", len(webapp.Agents))
	}
}

func TestGetPresetNotFound(t *testing.T) {
	if GetPreset("nonexistent") != nil {
		t.Error("expected nil for nonexistent preset")
	}
}

func TestPresetAgentItems(t *testing.T) {
	p := *GetPreset("Minimal")
	items := PresetAgentItems(p)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	for _, item := range items {
		if !item.enabled {
			t.Errorf("expected all items enabled, %s is not", item.agent.Name)
		}
		if item.agent.AutoTalk {
			t.Errorf("preset agent %s must default to AutoTalk=false", item.agent.Name)
		}
	}
}
