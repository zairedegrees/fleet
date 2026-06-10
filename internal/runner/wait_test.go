package runner

import (
	"testing"
	"time"
)

// waitGone replaces the blind 3s sleep in `fleet stop`: it returns as soon as the
// session is gone, and reports false if it never exits within the attempts.
func TestWaitGone(t *testing.T) {
	// becomes gone on the 3rd check → should return true
	n := 0
	if !waitGone(func() bool { n++; return n >= 3 }, 5, time.Millisecond) {
		t.Error("expected gone=true once the session disappears")
	}
	// never gone within the attempt budget → false
	if waitGone(func() bool { return false }, 3, time.Millisecond) {
		t.Error("expected gone=false when the session never exits")
	}
}
