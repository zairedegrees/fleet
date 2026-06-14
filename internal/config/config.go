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

// DefaultRelayURL is the wrai.th relay every component falls back to when no
// flag or config provides one.
const DefaultRelayURL = "http://localhost:8090/mcp"

type FleetConfig struct {
	Project ProjectConfig `toml:"project"`
	Claude  ClaudeConfig  `toml:"claude"`
	Agents  []AgentConfig `toml:"agents"`
}

type ProjectConfig struct {
	Name     string `toml:"name"`
	RelayURL string `toml:"relay_url"`
	Cwd      string `toml:"cwd"`
	// RelayBackend selects the coordination backend: "embedded" (the native
	// in-binary coord) or "download" (the AGPL agent-relay binary). Empty uses
	// the built-in default.
	RelayBackend string `toml:"relay_backend,omitempty"`
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

// unsafeRoleChars are the only characters forbidden in a free-text agent role:
// those that can break out of the quoting in the sinks the role reaches
// (single/double-quoted bash, tmux send-keys, curl JSON). Everything else —
// including accents, em-dashes, "&" and ordinary punctuation — stays inert
// inside the quotes and is allowed, so international prose roles work.
const unsafeRoleChars = "\"'`$\\\n\r\t"

var validColors = map[string]bool{
	"green": true, "orange": true, "blue": true, "red": true,
	"purple": true, "pink": true, "cyan": true, "yellow": true,
}

// NormalizeProjectName maps an arbitrary path basename to a name that satisfies
// validName: any character outside [a-zA-Z0-9_-] becomes '-', then leading
// non-alphanumeric and trailing '-' are trimmed. Keeps benign folders like
// "site.com" or "My App" usable instead of failing Validate().
func NormalizeProjectName(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	// validName requires the first character to be alphanumeric.
	return strings.TrimRight(strings.TrimLeft(b.String(), "-_"), "-")
}

func (cfg *FleetConfig) Validate() error {
	if cfg.Project.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if !validName.MatchString(cfg.Project.Name) {
		return fmt.Errorf("invalid project name %q: must be alphanumeric with hyphens/underscores", cfg.Project.Name)
	}
	if len(cfg.Agents) == 0 {
		return fmt.Errorf("at least one agent is required")
	}

	seen := make(map[string]bool)
	for _, a := range cfg.Agents {
		if !validName.MatchString(a.Name) {
			return fmt.Errorf("invalid agent name %q: must be alphanumeric with hyphens/underscores", a.Name)
		}
		if strings.ContainsAny(a.Role, unsafeRoleChars) {
			return fmt.Errorf("invalid role %q for agent %q: contains unsafe characters", a.Role, a.Name)
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
