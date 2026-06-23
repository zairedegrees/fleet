package runner

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

// A "bounded"-posture agent must NOT be woken at boot — the supervisor wakes it
// on its own cadence. Only "always" greets at boot. Posture is set directly here
// (AutoTalk unset), the shape a wizard-created config has, so the gate must read
// the posture, not the legacy AutoTalk mirror.
func TestBoundedDoesNotGreetAtBoot(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "p"},
		Agents: []config.AgentConfig{
			{Name: "always1", Color: "green", Posture: config.PostureAlways},
			{Name: "bound1", Color: "blue", Posture: config.PostureBounded},
			{Name: "idle1", Color: "red", Posture: config.PostureIdle},
		},
	}
	script := buildConfigureScript(cfg, "/tmp/x.log")

	alwaysLine := "tmux send-keys -t " + SessionName("p", "always1") + " '/relay talk'"
	if !strings.Contains(script, alwaysLine) {
		t.Errorf("always agent must be woken at boot; missing:\n%s", alwaysLine)
	}
	boundLine := "tmux send-keys -t " + SessionName("p", "bound1") + " '/relay talk'"
	if strings.Contains(script, boundLine) {
		t.Errorf("bounded agent must NOT be woken at boot (supervisor handles it); script wrongly contains:\n%s", boundLine)
	}
	idleLine := "tmux send-keys -t " + SessionName("p", "idle1") + " '/relay talk'"
	if strings.Contains(script, idleLine) {
		t.Errorf("idle agent must NOT be woken at boot; script wrongly contains:\n%s", idleLine)
	}
}
