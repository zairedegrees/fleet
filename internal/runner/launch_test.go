package runner

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

// stubExecHasSession records argv like stubExec but lets has-session report a
// chosen result, so CreateSessions proceeds to launch instead of skipping every
// agent (a real `true` always exits 0 → HasSession=true → skip).
func stubExecHasSession(t *testing.T, running bool) *[][]string {
	t.Helper()
	calls := &[][]string{}
	orig := execCommand
	execCommand = func(name string, arg ...string) *exec.Cmd {
		*calls = append(*calls, append([]string{name}, arg...))
		if len(arg) > 0 && arg[0] == "has-session" && !running {
			return exec.Command("false")
		}
		return exec.Command("true")
	}
	t.Cleanup(func() { execCommand = orig })
	return calls
}

// sentClaudeCmd reports whether a `tmux send-keys ... <want> Enter` was recorded.
func sentClaudeCmd(calls [][]string, want string) bool {
	for _, c := range calls {
		if len(c) >= 6 && c[1] == "send-keys" && c[4] == want {
			return true
		}
	}
	return false
}

// CreateSessions must launch each agent with its OWN BuildLaunch line (per-agent
// model/persona/tools), and write the persona file — not the one pre-loop command
// shared by all agents. A zero-behavioral agent stays byte-identical to v0.1.2.
func TestCreateSessionsLaunchesPerAgentBuildLaunch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	calls := stubExecHasSession(t, false)

	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: "/tmp/wd"},
		Claude:  config.ClaudeConfig{Flags: []string{"--dangerously-skip-permissions"}},
		Agents: []config.AgentConfig{
			{Name: "auditor", Color: "red", Role: "Review", Model: "opus", PermissionMode: "plan",
				Persona: "You are the auditor.\nSay \"no\" when unsure.", Tools: []string{"Read", "Grep"}},
			{Name: "dev", Color: "green", Role: "Dev"},
		},
	}

	for _, r := range CreateSessions(cfg, claudeBin) {
		if !r.Success {
			t.Fatalf("agent %s failed: %v", r.Agent, r.Error)
		}
	}

	// Persona file written for the auditor, none for the zero-behavioral dev.
	apath := personaFilePath("proj", "auditor")
	if b, err := os.ReadFile(apath); err != nil || !strings.Contains(string(b), "You are the auditor.") {
		t.Errorf("auditor persona file missing/wrong (%v)", err)
	}
	if _, err := os.Stat(personaFilePath("proj", "dev")); !os.IsNotExist(err) {
		t.Errorf("dev (no persona) must have no persona file")
	}

	wantAuditor := BuildLaunch(claudeBin, cfg.Claude.Flags, cfg.Agents[0], apath)
	wantDev := BuildLaunch(claudeBin, cfg.Claude.Flags, cfg.Agents[1], "")
	if !sentClaudeCmd(*calls, wantAuditor) {
		t.Errorf("auditor launch line not sent; want %q\ncalls: %v", wantAuditor, *calls)
	}
	if !sentClaudeCmd(*calls, wantDev) {
		t.Errorf("dev launch line not sent; want %q (must be byte-identical to v0.1.2)", wantDev)
	}
}

// The configure script (rename/color/wake) must never carry persona prose — the
// persona reaches the agent only via the launch file flag. A leak here would put
// a multiline prompt through tmux send-keys, the exact hazard we designed out.
func TestBuildConfigureScriptNeverContainsPersona(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "auditor", Color: "red", Role: "Review", Persona: "SENTINEL-PERSONA-PROSE", AutoTalk: true},
		},
	}
	script := buildConfigureScript(cfg, "/tmp/x.log")
	if strings.Contains(script, "SENTINEL-PERSONA-PROSE") {
		t.Errorf("persona prose leaked into the configure script:\n%s", script)
	}
}

func TestPersonaFilePath(t *testing.T) {
	p := personaFilePath("my-proj", "auditor")
	if !strings.HasSuffix(p, "/.fleet/personas/my-proj-auditor.txt") {
		t.Errorf("unexpected persona path: %q", p)
	}
}

func TestWritePersonaFileSkipsAgentWithoutPersona(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := writePersonaFile("proj", config.AgentConfig{Name: "dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("agent without persona must yield empty path, got %q", path)
	}
}

func TestWritePersonaFileRoundTripsBytesLosslessly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Every character that the launch escaping is afraid of, including a newline.
	persona := "You are the auditor.\nLoyalty is to correctness — say \"no\" when unsure.\nRun `go test`; check $PATH before you claim done."
	path, err := writePersonaFile("proj", config.AgentConfig{Name: "auditor", Persona: persona})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !strings.HasSuffix(path, "/.fleet/personas/proj-auditor.txt") {
		t.Errorf("unexpected path: %q", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if string(got) != persona {
		t.Errorf("persona bytes corrupted on disk:\n got: %q\nwant: %q", got, persona)
	}
}

const claudeBin = "/usr/local/bin/claude"

func TestBuildLaunch(t *testing.T) {
	tests := []struct {
		name        string
		global      []string
		agent       config.AgentConfig
		personaPath string
		want        string
	}{
		{
			name:  "zero-behavioral, no global flags — byte-identical to v0.1.2",
			agent: config.AgentConfig{Name: "dev"},
			want:  claudeBin,
		},
		{
			name:   "zero-behavioral with global skip — today's exact string",
			global: []string{"--dangerously-skip-permissions"},
			agent:  config.AgentConfig{Name: "dev"},
			want:   claudeBin + " --dangerously-skip-permissions",
		},
		{
			name:  "model only",
			agent: config.AgentConfig{Name: "dev", Model: "opus"},
			want:  claudeBin + " --model opus",
		},
		{
			name:  "permission only",
			agent: config.AgentConfig{Name: "dev", PermissionMode: "plan"},
			want:  claudeBin + " --permission-mode plan",
		},
		{
			name:        "persona path is single-quoted",
			agent:       config.AgentConfig{Name: "auditor", Persona: "irrelevant prose"},
			personaPath: "/home/u/.fleet/personas/proj-auditor.txt",
			want:        claudeBin + " --append-system-prompt-file '/home/u/.fleet/personas/proj-auditor.txt'",
		},
		{
			name:  "tools quoted as one value with relay rule appended",
			agent: config.AgentConfig{Name: "dev", Tools: []string{"Read", "Bash(go test:*)"}},
			want:  claudeBin + " --allowedTools 'Read,Bash(go test:*),mcp__agent-relay__*'",
		},
		{
			name:        "all combined, deterministic order",
			global:      []string{"--dangerously-skip-permissions"},
			agent:       config.AgentConfig{Name: "a", Model: "sonnet", PermissionMode: "acceptEdits", Persona: "p", Tools: []string{"Read"}},
			personaPath: "/p/proj-a.txt",
			want:        claudeBin + " --dangerously-skip-permissions --model sonnet --permission-mode acceptEdits --append-system-prompt-file '/p/proj-a.txt' --allowedTools 'Read,mcp__agent-relay__*'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildLaunch(claudeBin, tt.global, tt.agent, tt.personaPath)
			if got != tt.want {
				t.Errorf("\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

// The persona PROSE must never ride the launch command line — only its file path.
// A newline in the prose would otherwise become an Enter that submits the line early.
func TestBuildLaunchNeverContainsPersonaProse(t *testing.T) {
	a := config.AgentConfig{Name: "auditor", Persona: "SENTINEL-PERSONA-PROSE\nsecond line with $danger `id`"}
	got := BuildLaunch(claudeBin, nil, a, "/x/proj-auditor.txt")
	if strings.Contains(got, "SENTINEL-PERSONA-PROSE") {
		t.Errorf("persona prose leaked into launch line:\n%s", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("launch line must be single-line, got newline:\n%s", got)
	}
}

// Passing --allowedTools switches Claude to allow-list mode, which would strip
// the agent's unattended mcp__agent-relay__* access and break task routing. The
// value must therefore always carry the relay rule.
func TestToolsValueAlwaysIncludesRelayRule(t *testing.T) {
	got := toolsValue([]string{"Read", "Edit"})
	if got != "Read,Edit,mcp__agent-relay__*" {
		t.Errorf("got %q, want relay rule appended", got)
	}
}

func TestToolsValueDoesNotDuplicateRelayRule(t *testing.T) {
	got := toolsValue([]string{"Read", "mcp__agent-relay__*"})
	if got != "Read,mcp__agent-relay__*" {
		t.Errorf("got %q, relay rule must not be duplicated", got)
	}
	if strings.Count(got, "mcp__agent-relay__*") != 1 {
		t.Errorf("relay rule appears %d times in %q, want 1", strings.Count(got, "mcp__agent-relay__*"), got)
	}
}
