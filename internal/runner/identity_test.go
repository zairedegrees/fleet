package runner

import (
	"strings"
	"testing"
)

// identityPreamble is the prose message that travels with every wake so a woken
// agent knows who it is and that it is ALREADY registered — it must never call
// register_agent (which would NULL its profile_slug on an older relay). The text
// must name the agent and project, carry the as:/project: usage, and forbid
// register_agent.
func TestIdentityPreambleContent(t *testing.T) {
	got := identityPreamble("dev", "proj")

	for _, want := range []string{
		"agent 'dev'",
		"project 'proj'",
		"as:'dev'",
		"project:'proj'",
		"Do NOT call register_agent",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("identityPreamble must contain %q; got:\n%s", want, got)
		}
	}

	// One line of prose: a `/relay …` skill command would hit the autocomplete
	// Enter-swallow, and a newline would submit mid-message.
	if strings.Contains(got, "\n") {
		t.Errorf("identityPreamble must be a single line; got:\n%s", got)
	}
	if strings.Contains(got, "/relay") {
		t.Errorf("identityPreamble must be prose, not a /relay skill command; got:\n%s", got)
	}
}
