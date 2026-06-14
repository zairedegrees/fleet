package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

// assertBashValid fails if `bash -n` rejects the script — catches broken
// shell-escaping (the identity preamble carries single quotes, an em-dash and
// other special chars).
func assertBashValid(t *testing.T, script string) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	f, err := os.CreateTemp(t.TempDir(), "script-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(script); err != nil {
		t.Fatal(err)
	}
	f.Close()
	out, err := exec.Command("bash", "-n", f.Name()).CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n rejected the generated script: %v\n%s\nscript:\n%s", err, out, script)
	}
}

// The auto `/relay talk` at boot is what defeats Axe 4 token-saving. It must be
// gated on the agent's AutoTalk knob, NOT on !IsExecutive (a field never set in
// production). Default agents (AutoTalk=false) stay idle until dispatch wakes them.
func TestBuildConfigureScriptGatesTalkOnAutoTalk(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "talker", Color: "green", Role: "Dev", AutoTalk: true},
			{Name: "idle", Color: "blue", Role: "Dev"}, // AutoTalk defaults to false
		},
	}

	script := buildConfigureScript(cfg, "/tmp/x.log")

	talkerLine := "tmux send-keys -t " + SessionName("proj", "talker") + " '/relay talk'"
	if !strings.Contains(script, talkerLine) {
		t.Errorf("AutoTalk=true agent should get auto /relay talk; script missing line:\n%s", talkerLine)
	}

	idleLine := "tmux send-keys -t " + SessionName("proj", "idle") + " '/relay talk'"
	if strings.Contains(script, idleLine) {
		t.Errorf("AutoTalk=false agent must NOT get auto /relay talk; script wrongly contains:\n%s", idleLine)
	}
}

// Profile + vault HTTP live in the typed relay.Client only. The script does the
// pane-only work (rename/color) and, for AutoTalk agents, the wake.
func TestBuildConfigureScriptKeepsProfileAndVaultOutOfScript(t *testing.T) {
	dir := t.TempDir()
	vaultShared := filepath.Join(dir, ".fleet", "vault", "shared")
	if err := os.MkdirAll(vaultShared, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultShared, "doc.md"), []byte("# doc"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: dir},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
	}

	script := buildConfigureScript(cfg, "/tmp/x.log")

	for _, banned := range []string{"register_profile", "set_memory", "ensure_profile"} {
		if strings.Contains(script, banned) {
			t.Errorf("profile/vault HTTP must stay in the typed client; found %q", banned)
		}
	}
	for _, kept := range []string{"wait_prompt", "/rename dev"} {
		if !strings.Contains(script, kept) {
			t.Errorf("pane-dependent work must stay in the script; missing %q", kept)
		}
	}
}

// The agent must NEVER do a destructive bare self-register: a `/relay register`
// in the pane makes the agent's LLM call register_agent WITHOUT profile_slug,
// and an old relay's full-replace UPDATE NULLs the slug — silently breaking
// dispatched-task routing. fleet registers each agent server-side (registerFleet),
// so the configure script must contain NEITHER the in-pane /relay register NOR a
// register_agent curl for any agent.
func TestBuildConfigureScriptNeverSelfRegisters(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", RelayURL: "http://relay.test:9999/mcp", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Développeur — auth & paiements", ReportsTo: "lead", AutoTalk: true},
			{Name: "lead", Color: "red", Role: "Tech Lead", IsExecutive: true},
		},
	}

	script := buildConfigureScript(cfg, "/tmp/x.log")

	// Ban the self-register MECHANISMS: the in-pane /relay register skill command
	// and the register_agent JSON-RPC curl. (The identity preamble mentions
	// "register_agent" as prose telling the agent NOT to call it — that's fine.)
	for _, banned := range []string{"/relay register", `"register_agent"`, "curl"} {
		if strings.Contains(script, banned) {
			t.Errorf("configure script must never make the agent self-register; found %q in:\n%s", banned, script)
		}
	}
}

// rename + color are pane-only configuration and must stay.
func TestBuildConfigureScriptKeepsRenameAndColor(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
	}
	session := SessionName("proj", "dev")

	script := buildConfigureScript(cfg, "/tmp/x.log")

	for _, want := range []string{
		"tmux send-keys -t " + session + " '/rename dev' Enter",
		"tmux send-keys -t " + session + " '/color green' Enter",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("configure script missing pane config line:\n%s\ngot:\n%s", want, script)
		}
	}
}

// The first command after the prompt appears must be preceded by a settle: a
// freshly-booted pane shows the ❯ prompt while Claude Code is still initializing
// (MCP servers, skills), so the first keystrokes are dropped. Without the settle
// the /rename silently vanishes while the later /color survives.
func TestBuildConfigureScriptSettlesBeforeFirstCommand(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green"}},
	}
	session := SessionName("proj", "dev")

	script := buildConfigureScript(cfg, "/tmp/x.log")

	want := "if wait_prompt " + session + " 90; then\n  sleep 3\n  tmux send-keys -t " + session + " '/rename dev' Enter"
	if !strings.Contains(script, want) {
		t.Errorf("configure script must settle after wait_prompt before the first command (else /rename is dropped); got:\n%s", script)
	}
}

// For an AutoTalk agent the identity preamble (prose, normal Enter) must be sent
// BEFORE /relay talk so the woken agent knows who it is and polls correctly
// without ever registering. For an idle agent neither the preamble nor talk appear.
func TestBuildConfigureScriptPreamblePrecedesTalk(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "talker", Color: "green", Role: "Dev", AutoTalk: true},
			{Name: "idle", Color: "blue", Role: "Dev"},
		},
	}

	script := buildConfigureScript(cfg, "/tmp/x.log")
	talker := SessionName("proj", "talker")
	idle := SessionName("proj", "idle")

	preambleMarker := "tmux send-keys -t " + talker + " 'You are agent '\\''talker'\\''"
	talkLine := "tmux send-keys -t " + talker + " '/relay talk'"
	preIdx := strings.Index(script, preambleMarker)
	talkIdx := strings.Index(script, talkLine)
	if preIdx == -1 {
		t.Fatalf("AutoTalk agent missing identity preamble send-keys; script:\n%s", script)
	}
	if talkIdx == -1 {
		t.Fatalf("AutoTalk agent missing /relay talk; script:\n%s", script)
	}
	if preIdx > talkIdx {
		t.Errorf("identity preamble must precede /relay talk (got preamble@%d, talk@%d)", preIdx, talkIdx)
	}

	// Idle agent: no preamble, no talk.
	if strings.Contains(script, "tmux send-keys -t "+idle+" 'You are agent ") {
		t.Errorf("idle agent must NOT get the identity preamble; script:\n%s", script)
	}
	if strings.Contains(script, "tmux send-keys -t "+idle+" '/relay talk'") {
		t.Errorf("idle agent must NOT get /relay talk; script:\n%s", script)
	}
}

// The identity preamble carries single quotes, an em-dash and a colon; a broken
// escape would make the detached script un-runnable. Pin syntactic validity.
func TestBuildConfigureScriptIsValidBash(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "talker", Color: "green", Role: "Dev", AutoTalk: true},
			{Name: "idle", Color: "blue", Role: "Dev"},
		},
	}
	assertBashValid(t, buildConfigureScript(cfg, "/tmp/x.log"))
}
