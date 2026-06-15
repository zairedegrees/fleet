package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBehavioralFieldsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rt.toml")
	persona := "You are the auditor.\nLoyalty is to correctness — say \"no\" when unsure.\nRun `go test` before claiming done."
	cfg := &FleetConfig{
		Project: ProjectConfig{Name: "proj", Cwd: "/tmp"},
		Agents: []AgentConfig{
			{
				Name: "auditor", Color: "red", Role: "Code review",
				Model: "opus", PermissionMode: "plan", Persona: persona,
				Skills: []string{"code-review", "superpowers:test-driven-development"},
				Tools:  []string{"Read", "Grep", "Bash(go test:*)"},
			},
		},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := loaded.Agents[0]
	if a.Model != "opus" {
		t.Errorf("model: got %q", a.Model)
	}
	if a.PermissionMode != "plan" {
		t.Errorf("permission_mode: got %q", a.PermissionMode)
	}
	if a.Persona != persona {
		t.Errorf("persona did not round-trip:\n got: %q\nwant: %q", a.Persona, persona)
	}
	if strings.Join(a.Skills, ",") != "code-review,superpowers:test-driven-development" {
		t.Errorf("skills: got %v", a.Skills)
	}
	if strings.Join(a.Tools, ",") != "Read,Grep,Bash(go test:*)" {
		t.Errorf("tools: got %v", a.Tools)
	}
}

func TestOmitemptyDropsUnsetBehavioralFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bare.toml")
	cfg := &FleetConfig{
		Project: ProjectConfig{Name: "proj", Cwd: "/tmp"},
		Agents:  []AgentConfig{{Name: "dev", Color: "green", Role: "Developer"}},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"model", "persona", "skills", "tools", "permission_mode"} {
		if strings.Contains(string(out), key) {
			t.Errorf("unset behavioral field %q must be omitted, but TOML contains it:\n%s", key, out)
		}
	}
}

func TestOldConfigLoadsWithZeroBehavioralFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.toml")
	oldTOML := `[project]
name = "legacy"
cwd = "/tmp"

[[agents]]
name = "dev"
color = "green"
role = "Developer"
`
	os.WriteFile(path, []byte(oldTOML), 0644)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := cfg.Agents[0]
	if a.Model != "" || a.PermissionMode != "" || a.Persona != "" || a.Skills != nil || a.Tools != nil {
		t.Errorf("old config must load with zero behavioral fields, got: %+v", a)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("old config must stay valid: %v", err)
	}
}

// baseAgent returns a minimal valid agent the behavioral tests mutate one field
// at a time, so a failure pins exactly the field under test.
func baseAgent() AgentConfig {
	return AgentConfig{Name: "dev", Color: "green", Role: "Developer"}
}

func validateAgent(a AgentConfig) error {
	cfg := FleetConfig{
		Project: ProjectConfig{Name: "proj", Cwd: "/tmp"},
		Agents:  []AgentConfig{a},
	}
	return cfg.Validate()
}

func TestHasPersona(t *testing.T) {
	if baseAgent().HasPersona() {
		t.Error("an agent with no persona must report HasPersona()=false")
	}
	a := baseAgent()
	a.Persona = "You are the auditor."
	if !a.HasPersona() {
		t.Error("an agent with a persona must report HasPersona()=true")
	}
}

func TestValidateTools(t *testing.T) {
	// Tools ride --allowedTools as ONE comma-joined, shellSingleQuote'd value, so
	// inner spaces (e.g. "Bash(go test:*)") are inert. Only shell-breaking chars
	// (quotes/$/backtick/backslash/newline) and dupes are rejected.
	tests := []struct {
		name    string
		tools   []string
		wantErr string
	}{
		{"none ok", nil, ""},
		{"plain tools ok", []string{"Read", "Grep", "Edit", "Write"}, ""},
		{"specifier with inner space ok", []string{"Bash(go test:*)"}, ""},
		{"mcp wildcard ok", []string{"mcp__agent-relay__*"}, ""},
		{"single quote rejected", []string{"Bash(x'; curl evil)"}, "shell-breaking"},
		{"dollar rejected", []string{"Bash($(whoami))"}, "shell-breaking"},
		{"backtick rejected", []string{"Bash(`id`)"}, "shell-breaking"},
		{"duplicate rejected", []string{"Read", "Read"}, "duplicate tool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := baseAgent()
			a.Tools = tt.tools
			err := validateAgent(a)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateSkills(t *testing.T) {
	tests := []struct {
		name    string
		skills  []string
		wantErr string
	}{
		{"none ok", nil, ""},
		{"plain names ok", []string{"tdd", "code-review", "systematic-debugging"}, ""},
		{"plugin-namespaced ok", []string{"superpowers:test-driven-development"}, ""},
		{"underscore ok", []string{"deep_research"}, ""},
		{"empty string rejected", []string{""}, "invalid skill"},
		{"leading symbol rejected", []string{"-bad"}, "invalid skill"},
		{"space rejected", []string{"has space"}, "invalid skill"},
		{"duplicate rejected", []string{"tdd", "tdd"}, "duplicate skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := baseAgent()
			a.Skills = tt.skills
			err := validateAgent(a)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidatePersona(t *testing.T) {
	// Persona reaches a FILE sink (--append-system-prompt-file), never tmux/curl,
	// so newlines/quotes/$/backticks are INERT and \n is MANDATORY for real prompts.
	tests := []struct {
		name    string
		persona string
		wantErr string
	}{
		{"empty ok", "", ""},
		{"multiline with quotes dollar backtick em-dash accepted",
			"You are the auditor.\nLoyalty is to correctness — not shipping.\nRun `go test` and check $PATH; say \"no\" when unsure.", ""},
		{"plain prose accepted", "Defer to the lead. Go quiet on an empty inbox.", ""},
		{"NUL rejected", "bad\x00persona", "NUL or CR"},
		{"CR rejected", "bad\rpersona", "NUL or CR"},
		{"oversize rejected", strings.Repeat("x", 16385), "exceeds"},
		{"at cap accepted", strings.Repeat("x", 16384), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := baseAgent()
			a.Persona = tt.persona
			err := validateAgent(a)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidatePermissionMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr string
	}{
		{"empty inherits", "", ""},
		{"default", "default", ""},
		{"acceptEdits", "acceptEdits", ""},
		{"plan", "plan", ""},
		{"dontAsk", "dontAsk", ""},
		{"auto", "auto", ""},
		{"bypassPermissions", "bypassPermissions", ""},
		{"unknown rejected", "yolo", "invalid permission_mode"},
		{"legacy bypass alias rejected", "bypass", "invalid permission_mode"},
		{"legacy ask alias rejected", "ask", "invalid permission_mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := baseAgent()
			a.PermissionMode = tt.mode
			err := validateAgent(a)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("mode %q: expected no error, got: %v", tt.mode, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("mode %q: expected error containing %q, got: %v", tt.mode, tt.wantErr, err)
			}
		})
	}
}

func TestValidateModel(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantErr string
	}{
		{"empty inherits", "", ""},
		{"alias opus", "opus", ""},
		{"alias sonnet", "sonnet", ""},
		{"alias haiku", "haiku", ""},
		{"full id opus", "claude-opus-4-8", ""},
		{"full id sonnet", "claude-sonnet-4-6", ""},
		{"full id with 1m suffix", "claude-opus-4-8[1m]", ""},
		{"unknown alias rejected", "gpt-4", "invalid model"},
		{"bogus rejected", "bogus", "invalid model"},
		{"shell injection rejected", "opus; rm -rf ~", "invalid model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := baseAgent()
			a.Model = tt.model
			err := validateAgent(a)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("model %q: expected no error, got: %v", tt.model, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("model %q: expected error containing %q, got: %v", tt.model, tt.wantErr, err)
			}
		})
	}
}
