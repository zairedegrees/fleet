package coord

import "testing"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := OpenStore(t.TempDir() + "/coord.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store)
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
