package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")

	cfg := &FleetConfig{
		Project: ProjectConfig{
			Name:     "test-project",
			RelayURL: "http://localhost:8090/mcp",
			Cwd:      "/tmp/test",
		},
		Claude: ClaudeConfig{
			Flags: []string{"--dangerously-skip-permissions"},
		},
		Agents: []AgentConfig{
			{
				Name:        "boss",
				Color:       "green",
				Role:        "Project lead",
				IsExecutive: true,
			},
			{
				Name:      "worker",
				Color:     "blue",
				Role:      "Developer",
				ReportsTo: "boss",
			},
		},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Project.Name != cfg.Project.Name {
		t.Errorf("project name: got %q, want %q", loaded.Project.Name, cfg.Project.Name)
	}
	if len(loaded.Agents) != 2 {
		t.Fatalf("agents count: got %d, want 2", len(loaded.Agents))
	}
	if loaded.Agents[1].ReportsTo != "boss" {
		t.Errorf("reports_to: got %q, want %q", loaded.Agents[1].ReportsTo, "boss")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     FleetConfig
		wantErr string
	}{
		{
			name: "valid config",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "my-project", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "dev", Color: "green", Role: "Developer"}},
			},
			wantErr: "",
		},
		{
			name: "empty project name",
			cfg: FleetConfig{
				Project: ProjectConfig{Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
			},
			wantErr: "project name is required",
		},
		{
			name: "invalid agent name",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "test", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "bad agent!", Color: "green", Role: "Dev"}},
			},
			wantErr: "invalid agent name",
		},
		{
			name: "invalid color",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "test", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "dev", Color: "rainbow", Role: "Dev"}},
			},
			wantErr: "invalid color",
		},
		{
			name: "duplicate agent names",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "test", Cwd: "/tmp"},
				Agents: []AgentConfig{
					{Name: "dev", Color: "green", Role: "Dev"},
					{Name: "dev", Color: "blue", Role: "Dev2"},
				},
			},
			wantErr: "duplicate agent name",
		},
		{
			name: "no agents",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "test", Cwd: "/tmp"},
			},
			wantErr: "at least one agent is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestAutoTalkBackwardCompat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.toml")

	oldTOML := `[project]
name = "legacy"
relay_url = "http://localhost:8090/mcp"
cwd = "/tmp"

[[agents]]
name = "dev"
color = "green"
role = "Developer"
`
	os.WriteFile(path, []byte(oldTOML), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Agents[0].AutoTalk {
		t.Error("expected AutoTalk to default to false for old configs")
	}
}

func TestAutoTalkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")

	cfg := &FleetConfig{
		Project: ProjectConfig{Name: "test", Cwd: "/tmp"},
		Agents: []AgentConfig{
			{Name: "boss", Color: "green", Role: "Lead", IsExecutive: true, AutoTalk: true},
			{Name: "worker", Color: "blue", Role: "Dev"},
		},
	}
	Save(path, cfg)
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !loaded.Agents[0].AutoTalk {
		t.Error("expected boss AutoTalk=true")
	}
	if loaded.Agents[1].AutoTalk {
		t.Error("expected worker AutoTalk=false")
	}
}

func TestLoadLastSymlink(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	os.MkdirAll(configDir, 0755)

	cfg := &FleetConfig{
		Project: ProjectConfig{Name: "my-project"},
	}

	configPath := filepath.Join(configDir, "my-project.toml")
	Save(configPath, cfg)

	lastPath := filepath.Join(dir, "last.toml")
	os.Symlink(configPath, lastPath)

	loaded, err := Load(lastPath)
	if err != nil {
		t.Fatalf("Load via symlink failed: %v", err)
	}
	if loaded.Project.Name != "my-project" {
		t.Errorf("got %q, want %q", loaded.Project.Name, "my-project")
	}
}
