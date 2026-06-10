package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

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

// The relay JSON-RPC protocol lives in ONE place: the typed relay.Client. The
// generated bash keeps only pane-dependent work (prompt wait + send-keys) —
// no curl, no hand-built JSON, no relay URL.
func TestBuildConfigureScriptContainsNoRelayHTTP(t *testing.T) {
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

	for _, banned := range []string{"curl", "jsonrpc", "RELAY_URL", "register_agent", "register_profile", "set_memory", "ensure_profile"} {
		if strings.Contains(script, banned) {
			t.Errorf("configure script must not speak relay HTTP; found %q", banned)
		}
	}
	for _, kept := range []string{"wait_prompt", "/rename dev", "/relay register"} {
		if !strings.Contains(script, kept) {
			t.Errorf("pane-dependent work must stay in the script; missing %q", kept)
		}
	}
}
