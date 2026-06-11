package runner

import (
	"strings"
	"testing"
	"time"
)

// WakeAgent must deliver the identity preamble (prose, single send-keys + Enter)
// BEFORE the /relay talk type → settle → SEPARATE Enter sequence, and must NEVER
// make the agent self-register: no register_agent, no /relay register. The
// has-session probe runs first (stubExec makes it succeed).
func TestWakeAgentSendsPreambleThenTalk(t *testing.T) {
	calls := stubExec(t)
	origSettle := submitSettle
	submitSettle = time.Millisecond
	t.Cleanup(func() { submitSettle = origSettle })

	if err := WakeAgent("proj", "dev"); err != nil {
		t.Fatalf("WakeAgent failed: %v", err)
	}

	session := SessionName("proj", "dev")
	want := [][]string{
		{"tmux", "has-session", "-t", session},
		{"tmux", "send-keys", "-t", session, identityPreamble("dev", "proj"), "Enter"},
		{"tmux", "send-keys", "-t", session, "/relay talk"},
		{"tmux", "send-keys", "-t", session, "Enter"},
	}
	assertCalls(t, *calls, want)

	// No self-register MECHANISM anywhere in the argv. (The preamble argv mentions
	// "register_agent" as prose forbidding it — assertCalls already pins the exact
	// 4-call set, so here we just guard the destructive /relay register command.)
	for _, c := range *calls {
		if strings.Contains(strings.Join(c, " "), "/relay register") {
			t.Errorf("WakeAgent must never make the agent self-register; saw argv: %v", c)
		}
	}
}
