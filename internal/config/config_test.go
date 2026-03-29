package config

import (
	"os"
	"path/filepath"
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
