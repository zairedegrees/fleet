package runner

import (
	"os/exec"
	"strings"
	"testing"
)

// A broken/absent tmux must be distinguishable from "0 sessions running", so the
// CLI can say "tmux not found" instead of silently reporting an empty fleet.
// The install hint must match the OS: brew is darwin-only.
func TestClassifyListErr(t *testing.T) {
	// tmux binary missing → real, actionable error with an OS-appropriate hint.
	if err := classifyListErr("darwin", exec.ErrNotFound, nil); err == nil || !strings.Contains(err.Error(), "brew install tmux") {
		t.Errorf("missing tmux on darwin should hint brew, got: %v", err)
	}
	if err := classifyListErr("linux", exec.ErrNotFound, nil); err == nil || strings.Contains(err.Error(), "brew") || !strings.Contains(err.Error(), "apt install tmux") {
		t.Errorf("missing tmux on linux should hint apt and never brew, got: %v", err)
	}
	if err := classifyListErr("freebsd", exec.ErrNotFound, nil); err == nil || strings.Contains(err.Error(), "brew") {
		t.Errorf("missing tmux off darwin/linux should never hint brew, got: %v", err)
	}
	// "no server running" → benign (no sessions), not an error.
	if err := classifyListErr("darwin", &exec.ExitError{}, []byte("no server running on /private/tmp/tmux-501/default")); err != nil {
		t.Errorf("'no server running' should be treated as no-sessions (nil), got: %v", err)
	}
	// any other tmux failure → real error.
	if err := classifyListErr("darwin", &exec.ExitError{}, []byte("server exited unexpectedly")); err == nil {
		t.Error("an unexpected tmux failure should surface as an error, got nil")
	}
}
