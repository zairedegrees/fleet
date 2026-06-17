package coord

import (
	"testing"
	"time"
)

func drainWakes(s *Server, d time.Duration) []WakeRequest {
	var out []WakeRequest
	deadline := time.After(d)
	for {
		select {
		case w := <-s.Dispatched():
			out = append(out, w)
		case <-deadline:
			return out
		}
	}
}

// dispatch_task emits a WakeRequest for each notified recipient (not the dispatcher).
func TestDispatchTaskEmitsWakeEvent(t *testing.T) {
	s := newTestServer(t)
	mustCall(t, s, "register_agent", map[string]any{"name": "worker", "project": "acme", "profile_slug": "dev"})
	mustCall(t, s, "register_agent", map[string]any{"name": "lead", "project": "acme", "profile_slug": "dev"})

	res, err := handleDispatchTask(s, map[string]any{
		"as": "lead", "project": "acme", "profile": "dev", "title": "do the thing",
	})
	if err != nil || res.IsError {
		t.Fatalf("dispatch failed: err=%v res=%+v", err, res)
	}

	got := drainWakes(s, 200*time.Millisecond)
	if len(got) != 1 || got[0].Project != "acme" || got[0].Agent != "worker" {
		t.Fatalf("want one wake for acme/worker, got %+v", got)
	}
}

// Emitting must never block dispatch even if nobody drains the channel.
func TestDispatchEmitNonBlocking(t *testing.T) {
	s := newTestServer(t)
	for i := 0; i < 1000; i++ {
		s.emitDispatched("acme", "worker") // far exceeds the buffer; must not block
	}
}
