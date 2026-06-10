package runner

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

// Every wake.sh wake (single-agent and --all) must deliver the identity preamble
// BEFORE /relay talk so the woken worker knows who it is and never self-registers
// (a bare register_agent drops profile_slug and breaks task routing on old relays).
func TestBuildWakeScriptPreamblePrecedesTalk(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Dev"},
			{Name: "lead", Color: "red", Role: "Lead", IsExecutive: true},
		},
	}

	script := buildWakeScript(cfg)

	// Workers are woken; the executive is never a wake target.
	if !strings.Contains(script, "You are agent '\\''dev'\\''") {
		t.Errorf("wake.sh missing identity preamble for worker dev:\n%s", script)
	}
	if strings.Contains(script, "You are agent '\\''lead'\\''") {
		t.Errorf("executive lead must not be a wake target:\n%s", script)
	}

	// In the --all loop and the single-agent path the preamble must come before
	// /relay talk. Check every preamble occurrence is followed by a talk.
	for _, marker := range []string{
		"You are agent '\\''dev'\\''", // --all worker
	} {
		preIdx := strings.Index(script, marker)
		if preIdx == -1 {
			t.Fatalf("missing preamble %q", marker)
		}
		rest := script[preIdx:]
		if !strings.Contains(rest, "/relay talk") {
			t.Errorf("preamble %q not followed by /relay talk", marker)
		}
	}

	// Single-agent path uses $SESSION and a double-quoted preamble so $1 expands;
	// the literal single quotes around the name stay plain. It must also precede talk.
	if !strings.Contains(script, `"You are agent '$1' on project 'proj'`) {
		t.Errorf("single-agent wake must send the identity preamble for $1:\n%s", script)
	}
	single := strings.Index(script, `"You are agent '$1'`)
	talkIdx := strings.LastIndex(script, "/relay talk")
	if single == -1 || talkIdx == -1 || single > talkIdx {
		t.Errorf("single-agent preamble must precede /relay talk (preamble@%d talk@%d)", single, talkIdx)
	}

	// Keep the existing UX: woken/no-session echoes survive.
	for _, want := range []string{"woken", "no session"} {
		if !strings.Contains(script, want) {
			t.Errorf("wake.sh must keep the %q UX:\n%s", want, script)
		}
	}
}

// wake.sh escaping must be valid bash — the preamble carries single quotes and
// an em-dash; a broken escape would make the boss's wake command un-runnable.
func TestBuildWakeScriptIsValidBash(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Dev"},
			{Name: "ops", Color: "blue", Role: "Ops"},
		},
	}
	assertBashValid(t, buildWakeScript(cfg))
}
