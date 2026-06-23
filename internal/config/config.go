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

// DefaultRelayURL is the local coordination endpoint every component falls back
// to when no flag or config provides one.
const DefaultRelayURL = "http://localhost:8090/mcp"

type FleetConfig struct {
	Project ProjectConfig `toml:"project"`
	Claude  ClaudeConfig  `toml:"claude"`
	Agents  []AgentConfig `toml:"agents"`

	// BoundedDefaults are the fleet-wide bounded-policy defaults applied to every
	// "bounded"-posture agent, under the built-in defaults and over by per-agent
	// [agents.bounded]. Nil when unset.
	BoundedDefaults *BoundedPolicy `toml:"bounded_defaults,omitempty"`
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

	// Posture replaces the legacy auto_talk boolean: "idle" (default, woken on
	// dispatch), "bounded" (proactive re-wake under a cap, driven by the
	// supervisor), or "always" (greets at boot). Normalize() maps a legacy
	// auto_talk into Posture and keeps auto_talk as a derived mirror.
	Posture string `toml:"posture,omitempty"`

	// Bounded overrides the fleet-wide bounded defaults for this agent. Only
	// meaningful when Posture == "bounded". Nil inherits.
	Bounded *BoundedPolicy `toml:"bounded,omitempty"`

	// Model selects the per-agent Claude model (--model). "" inherits Claude's
	// default. Lands on the unquoted launch argv, so it must be allowlist-known
	// and shell-safe (validated in Validate).
	Model string `toml:"model,omitempty"`

	// PermissionMode selects the per-agent permission posture (--permission-mode).
	// "" inherits. Ignored at launch when the fleet-wide skip-all flag is set.
	PermissionMode string `toml:"permission_mode,omitempty"`

	// Persona is a multiline system prompt injected at launch via a FILE
	// (--append-system-prompt-file), never inlined into a shell or tmux send-keys.
	// Because the sink is a file, newlines/quotes/$/backticks are inert and a
	// newline is expected; only NUL/CR are forbidden (validated in Validate).
	Persona string `toml:"persona,omitempty"`

	// Skills is advisory metadata only: there is no CLI flag to preload skills,
	// so this never alters the launched process. It is mirrored to the relay
	// profile so peers can discover what an agent is meant to do.
	Skills []string `toml:"skills,omitempty"`

	// Tools is a single allow-list passed to --allowedTools. The launch always
	// re-adds the relay rule so a narrowed scope never strips task routing. The
	// whole value is comma-joined and shell-quoted as one argument, so inner
	// spaces (e.g. "Bash(go test:*)") are safe; only shell-breaking chars are
	// rejected (validated in Validate).
	Tools []string `toml:"tools,omitempty"`
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// HasPersona reports whether a persona file must be written for this agent at
// launch (the launch skips --append-system-prompt-file when false).
func (a AgentConfig) HasPersona() bool { return a.Persona != "" }

// unsafeRoleChars are the only characters forbidden in a free-text agent role:
// those that can break out of the quoting in the sinks the role reaches
// (single/double-quoted bash, tmux send-keys, curl JSON). Everything else —
// including accents, em-dashes, "&" and ordinary punctuation — stays inert
// inside the quotes and is allowed, so international prose roles work.
const unsafeRoleChars = "\"'`$\\\n\r\t"

// unsafePersonaChars are the only bytes forbidden in a persona. Unlike a Role,
// a Persona is written to a FILE and passed to claude as a path
// (--append-system-prompt-file), so quotes/$/backticks/newlines are inert and a
// newline is required for real prompts. Only NUL and CR are rejected.
const unsafePersonaChars = "\x00\r"

// maxPersonaBytes caps a persona: it's a system prompt, not a document. Anything
// larger is almost certainly a paste mistake.
const maxPersonaBytes = 16384

var validColors = map[string]bool{
	"green": true, "orange": true, "blue": true, "red": true,
	"purple": true, "pink": true, "cyan": true, "yellow": true,
}

// validModelAlias holds the short model names accepted on [[agents]] model.
// "" means inherit Claude's default. Full model ids are accepted separately via
// validModelID so new releases work without a code change.
var validModelAlias = map[string]bool{
	"": true, "opus": true, "sonnet": true, "haiku": true, "fable": true,
}

// validModelID matches a fully-qualified Claude model id (e.g. claude-opus-4-8,
// claude-sonnet-4-6, claude-opus-4-8[1m]) so forward model ids are accepted
// without an allowlist bump. Lowercase, digits, dots, hyphens and the [..] suffix.
var validModelID = regexp.MustCompile(`^claude-[a-z0-9.\[\]-]+$`)

// validPermissionModes is the exact set of --permission-mode values Claude Code
// accepts, plus "" (inherit). Legacy aliases like "bypass"/"ask" are NOT valid.
var validPermissionModes = map[string]bool{
	"": true, "default": true, "acceptEdits": true, "auto": true,
	"plan": true, "dontAsk": true, "bypassPermissions": true,
}

// validSkillName is validName plus ':' for plugin-namespaced skills
// (e.g. superpowers:test-driven-development).
var validSkillName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_:-]*$`)

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
		// Model is the one behavioral field that reaches the unquoted launch argv
		// (--model <id>), so it must be both allowlist-known and shell-safe.
		if a.Model != "" {
			if !validModelAlias[a.Model] && !validModelID.MatchString(a.Model) {
				return fmt.Errorf("invalid model %q for agent %q", a.Model, a.Name)
			}
			if strings.ContainsAny(a.Model, unsafeRoleChars+" \t") {
				return fmt.Errorf("model %q for agent %q contains unsafe characters", a.Model, a.Name)
			}
		}
		if !validPermissionModes[a.PermissionMode] {
			return fmt.Errorf("invalid permission_mode %q for agent %q", a.PermissionMode, a.Name)
		}
		// Persona reaches a file sink: \n and quotes are SAFE. Do NOT route it
		// through unsafeRoleChars (that bans \n, which every persona needs).
		if strings.ContainsAny(a.Persona, unsafePersonaChars) {
			return fmt.Errorf("persona for agent %q contains NUL or CR", a.Name)
		}
		if len(a.Persona) > maxPersonaBytes {
			return fmt.Errorf("persona for agent %q exceeds %d bytes", a.Name, maxPersonaBytes)
		}
		if a.Posture != "" && !validPostures[a.Posture] {
			return fmt.Errorf("invalid posture %q for agent %q", a.Posture, a.Name)
		}
		if a.Bounded != nil {
			if err := a.Bounded.Validate(); err != nil {
				return fmt.Errorf("agent %q bounded policy: %w", a.Name, err)
			}
		}
		// Skills never reach a shell — name grammar + no empties/dupes is enough.
		seenSkill := make(map[string]bool)
		for _, s := range a.Skills {
			if !validSkillName.MatchString(s) {
				return fmt.Errorf("invalid skill %q for agent %q", s, a.Name)
			}
			if seenSkill[s] {
				return fmt.Errorf("duplicate skill %q for agent %q", s, a.Name)
			}
			seenSkill[s] = true
		}
		// Tools ride --allowedTools as one comma-joined, shell-quoted value, so
		// inner spaces are inert (unsafeRoleChars excludes space); reject only the
		// chars that would break out of the surrounding quotes.
		seenTool := make(map[string]bool)
		for _, tool := range a.Tools {
			if strings.ContainsAny(tool, unsafeRoleChars) {
				return fmt.Errorf("invalid tool %q for agent %q: contains shell-breaking chars", tool, a.Name)
			}
			if seenTool[tool] {
				return fmt.Errorf("duplicate tool %q for agent %q", tool, a.Name)
			}
			seenTool[tool] = true
		}
		if seen[a.Name] {
			return fmt.Errorf("duplicate agent name %q", a.Name)
		}
		seen[a.Name] = true
	}
	if cfg.BoundedDefaults != nil {
		if err := cfg.BoundedDefaults.Validate(); err != nil {
			return fmt.Errorf("bounded_defaults: %w", err)
		}
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
	cfg.Normalize()
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
