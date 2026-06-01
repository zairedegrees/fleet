package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

type FleetConfig struct {
	Project ProjectConfig `toml:"project"`
	Claude  ClaudeConfig  `toml:"claude"`
	Agents  []AgentConfig `toml:"agents"`
}

type ProjectConfig struct {
	Name     string `toml:"name"`
	RelayURL string `toml:"relay_url"`
	Cwd      string `toml:"cwd"`
}

type ClaudeConfig struct {
	Bin   string   `toml:"bin,omitempty"`
	Flags []string `toml:"flags"`
}

// ResolveBin returns the absolute path to the Claude Code binary fleet should
// launch. If Bin is set it is used (supporting ~ expansion and explicit paths);
// otherwise "claude" is looked up on PATH. Resolving to an absolute path here,
// in fleet's own environment which carries the user's shell PATH, means the
// spawned tmux shells do not need claude on their own PATH.
func (c ClaudeConfig) ResolveBin() (string, error) {
	bin := c.Bin
	if bin == "" {
		bin = "claude"
	}
	if strings.HasPrefix(bin, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			bin = filepath.Join(home, bin[2:])
		}
	}
	if strings.Contains(bin, "/") {
		info, err := os.Stat(bin)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("claude binary not found at %q", bin)
		}
		return bin, nil
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("claude binary %q not found on PATH (install Claude Code, or set [claude] bin in the config)", bin)
	}
	return path, nil
}

type AgentConfig struct {
	Name        string `toml:"name"`
	Color       string `toml:"color"`
	Role        string `toml:"role"`
	ReportsTo   string `toml:"reports_to,omitempty"`
	IsExecutive bool   `toml:"is_executive,omitempty"`
	AutoTalk    bool   `toml:"auto_talk,omitempty"`
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var validColors = map[string]bool{
	"green": true, "orange": true, "blue": true, "red": true,
	"purple": true, "pink": true, "cyan": true, "yellow": true,
}

func (cfg *FleetConfig) Validate() error {
	if cfg.Project.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if len(cfg.Agents) == 0 {
		return fmt.Errorf("at least one agent is required")
	}

	seen := make(map[string]bool)
	for _, a := range cfg.Agents {
		if !validName.MatchString(a.Name) {
			return fmt.Errorf("invalid agent name %q: must be alphanumeric with hyphens/underscores", a.Name)
		}
		if !validColors[a.Color] {
			return fmt.Errorf("invalid color %q for agent %q", a.Color, a.Name)
		}
		if seen[a.Name] {
			return fmt.Errorf("duplicate agent name %q", a.Name)
		}
		seen[a.Name] = true
	}
	return nil
}

func FleetDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fleet")
}

func Save(path string, cfg *FleetConfig) error {
	os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func Load(path string) (*FleetConfig, error) {
	var cfg FleetConfig
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveAsLast(cfg *FleetConfig) error {
	dir := FleetDir()
	configDir := filepath.Join(dir, "configs")
	os.MkdirAll(configDir, 0755)

	configPath := filepath.Join(configDir, cfg.Project.Name+".toml")
	if err := Save(configPath, cfg); err != nil {
		return err
	}

	lastPath := filepath.Join(dir, "last.toml")
	os.Remove(lastPath)
	return os.Symlink(configPath, lastPath)
}

func LoadLast() (*FleetConfig, error) {
	return Load(filepath.Join(FleetDir(), "last.toml"))
}
