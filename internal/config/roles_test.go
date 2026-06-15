package config

import "testing"

func TestDefaultModelForRole(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"architect", "opus"},
		{"auditor", "opus"},
		{"researcher", "opus"},
		{"quant", "opus"},
		{"security", "opus"},
		{"dev", "sonnet"},
		{"frontend", "sonnet"},
		{"ux-designer", "sonnet"},
		{"ops", "sonnet"},
		{"docs", "sonnet"},
		{"notifier", "haiku"},
		{"whatever-new-role", "sonnet"}, // unknown → safe builder default
	}
	for _, tt := range tests {
		if got := DefaultModelForRole(tt.role); got != tt.want {
			t.Errorf("DefaultModelForRole(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

func TestResolveDefaults(t *testing.T) {
	// A known role with no behavioral fields gets the whole role profile.
	got := ResolveDefaults(AgentConfig{Name: "auditor", Color: "red"})
	if got.Model != "opus" || got.PermissionMode != "plan" || got.Persona == "" || len(got.Skills) == 0 || len(got.Tools) == 0 {
		t.Errorf("bare known role must be fully resolved, got %+v", got)
	}

	// Explicit values win; only empty fields are filled.
	custom := ResolveDefaults(AgentConfig{Name: "dev", Color: "green", Model: "haiku", Persona: "MINE"})
	if custom.Model != "haiku" {
		t.Errorf("explicit model must win, got %q", custom.Model)
	}
	if custom.Persona != "MINE" {
		t.Errorf("explicit persona must win, got %q", custom.Persona)
	}
	if custom.PermissionMode != "acceptEdits" { // was empty → filled from dev profile
		t.Errorf("empty permission must be filled, got %q", custom.PermissionMode)
	}

	// An unknown role is returned untouched (custom agents stay bare).
	unknown := ResolveDefaults(AgentConfig{Name: "loissuisses-indexer", Color: "blue"})
	if unknown.Model != "" || unknown.Persona != "" || unknown.Tools != nil {
		t.Errorf("unknown role must be unchanged, got %+v", unknown)
	}
}
