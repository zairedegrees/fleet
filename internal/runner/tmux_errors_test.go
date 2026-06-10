package runner

import (
	"os/exec"
	"strings"
	"testing"
)

// A broken/absent tmux must be distinguishable from "0 sessions running", so the
// CLI can say "tmux not found" instead of silently reporting an empty fleet.
func TestClassifyListErr(t *testing.T) {
	// tmux binary missing → real, actionable error.
	if err := classifyListErr(exec.ErrNotFound, nil); err == nil || !strings.Contains(err.Error(), "tmux") {
		t.Errorf("missing tmux should yield a tmux error, got: %v", err)
	}
	// "no server running" → benign (no sessions), not an error.
	if err := classifyListErr(&exec.ExitError{}, []byte("no server running on /private/tmp/tmux-501/default")); err != nil {
		t.Errorf("'no server running' should be treated as no-sessions (nil), got: %v", err)
	}
	// any other tmux failure → real error.
	if err := classifyListErr(&exec.ExitError{}, []byte("server exited unexpectedly")); err == nil {
		t.Error("an unexpected tmux failure should surface as an error, got nil")
	}
}
