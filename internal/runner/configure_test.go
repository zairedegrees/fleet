package runner

import (
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

	script := buildConfigureScript(cfg, "http://localhost:8090/mcp", "/tmp/x.log")

	talkerLine := "tmux send-keys -t " + SessionName("proj", "talker") + " '/relay talk'"
	if !strings.Contains(script, talkerLine) {
		t.Errorf("AutoTalk=true agent should get auto /relay talk; script missing line:\n%s", talkerLine)
	}

	idleLine := "tmux send-keys -t " + SessionName("proj", "idle") + " '/relay talk'"
	if strings.Contains(script, idleLine) {
		t.Errorf("AutoTalk=false agent must NOT get auto /relay talk; script wrongly contains:\n%s", idleLine)
	}
}
