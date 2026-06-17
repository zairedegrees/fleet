package coord

import "testing"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(newTestStore(t))
}

func TestRegisterNotifyChannelRoundTrip(t *testing.T) {
	s := newTestServer(t)
	res, err := handleRegisterNotifyChannel(s, map[string]any{
		"project": "acme", "name": "dev", "target": "tmux:fleet-acme-dev",
	})
	if err != nil || res.IsError {
		t.Fatalf("register failed: err=%v res=%+v", err, res)
	}
	target, ok, err := s.NotifyChannelTarget("acme", "dev")
	if err != nil {
		t.Fatalf("lookup err: %v", err)
	}
	if !ok || target != "tmux:fleet-acme-dev" {
		t.Fatalf("got target=%q ok=%v, want tmux:fleet-acme-dev/true", target, ok)
	}
}

func TestRegisterNotifyChannelRequiresFields(t *testing.T) {
	s := newTestServer(t)
	if res, _ := handleRegisterNotifyChannel(s, map[string]any{"project": "acme", "target": "tmux:x"}); !res.IsError {
		t.Error("missing name must error")
	}
	if res, _ := handleRegisterNotifyChannel(s, map[string]any{"project": "acme", "name": "dev"}); !res.IsError {
		t.Error("missing target must error")
	}
}

func TestRegisterNotifyChannelRejectsNonTmuxTarget(t *testing.T) {
	s := newTestServer(t)
	res, err := handleRegisterNotifyChannel(s, map[string]any{
		"project": "acme", "name": "dev", "target": "http://example.com",
	})
	if err != nil || !res.IsError {
		t.Fatalf("non-tmux target must error, got err=%v res=%+v", err, res)
	}
	if _, ok, err := s.NotifyChannelTarget("acme", "dev"); err != nil || ok {
		t.Fatalf("rejected target must not be stored, got ok=%v err=%v", ok, err)
	}
}

func TestRegisterNotifyChannelFoldsName(t *testing.T) {
	s := newTestServer(t)
	res, err := handleRegisterNotifyChannel(s, map[string]any{
		"project": "acme", "name": "Dev", "target": "tmux:fleet-acme-dev",
	})
	if err != nil || res.IsError {
		t.Fatalf("register failed: err=%v res=%+v", err, res)
	}
	// register_agent stores the lowercased name; the waker looks up "dev".
	target, ok, err := s.NotifyChannelTarget("acme", "dev")
	if err != nil || !ok || target != "tmux:fleet-acme-dev" {
		t.Fatalf("case-folded lookup failed: target=%q ok=%v err=%v", target, ok, err)
	}
}

func TestNotifyChannelMissingIsNotFound(t *testing.T) {
	s := newTestServer(t)
	if _, ok, err := s.NotifyChannelTarget("acme", "ghost"); err != nil || ok {
		t.Fatalf("missing channel must be (_, false, nil), got ok=%v err=%v", ok, err)
	}
}

func TestRegisterNotifyChannelIsOperatorOnly(t *testing.T) {
	if _, ok := handlers["register_notify_channel"]; !ok {
		t.Error("register_notify_channel must be in the handlers map")
	}
	for _, td := range advertisedTools() {
		if td.Name == "register_notify_channel" {
			t.Error("register_notify_channel must NOT be advertised")
		}
	}
}
