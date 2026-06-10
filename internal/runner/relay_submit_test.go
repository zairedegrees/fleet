package runner

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

// A `/relay` slash command must be submitted by a SEPARATE Enter. Sent as
// '<cmd>' Enter in a single send-keys, Claude Code types the text but the Enter
// is swallowed by the skill autocomplete, so the command is never run — the
// agent never registers its role.
func TestBuildConfigureScriptSubmitsRelayRegisterWithSeparateEnter(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
	}
	script := buildConfigureScript(cfg, "/tmp/x.log")
	session := SessionName("proj", "dev")

	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, "/relay register") && strings.HasSuffix(line, "Enter") {
			t.Errorf("/relay register combines text + Enter (won't submit): %q", line)
		}
	}
	if !strings.Contains(script, "tmux send-keys -t "+session+" Enter\n") {
		t.Error("expected a separate Enter send-key to submit /relay register")
	}
}
