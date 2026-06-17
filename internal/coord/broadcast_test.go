package coord

import "testing"

func inboxCount(t *testing.T, s *Server, agent string) int {
	t.Helper()
	res := mustCall(t, s, "get_inbox", map[string]any{"project": "p", "as": agent})
	var ib struct {
		Count int `json:"count"`
	}
	decodePayload(t, res, &ib)
	return ib.Count
}

func TestExecutiveCanBroadcast(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"project": "p", "name": "lead", "is_executive": true})
	mustCall(t, s, "register_agent", map[string]any{"project": "p", "name": "dev"})
	res := mustCall(t, s, "send_message", map[string]any{
		"project": "p", "as": "lead", "to": "*", "content": "all hands",
	})
	if res.IsError {
		t.Fatalf("executive broadcast should succeed: %s", res.Content[0].Text)
	}
	if n := inboxCount(t, s, "dev"); n != 1 {
		t.Errorf("dev should have received the broadcast, got %d", n)
	}
}

func TestNonExecutiveCannotBroadcast(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"project": "p", "name": "dev"})
	mustCall(t, s, "register_agent", map[string]any{"project": "p", "name": "auditor"})
	res := callTool(t, s, "send_message", map[string]any{
		"project": "p", "as": "dev", "to": "*", "content": "hey all",
	})
	if !res.IsError {
		t.Fatal("non-executive broadcast must be rejected")
	}
	if n := inboxCount(t, s, "auditor"); n != 0 {
		t.Errorf("rejected broadcast must deliver nothing, got %d", n)
	}
}

func TestAnonymousCannotBroadcast(t *testing.T) {
	s := New(newTestStore(t))
	res := callTool(t, s, "send_message", map[string]any{"project": "p", "to": "*", "content": "hi"})
	if !res.IsError {
		t.Fatal("anonymous broadcast must be rejected")
	}
}

func TestDirectMessageNotGated(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"project": "p", "name": "dev"})
	mustCall(t, s, "register_agent", map[string]any{"project": "p", "name": "auditor"})
	res := mustCall(t, s, "send_message", map[string]any{
		"project": "p", "as": "dev", "to": "auditor", "content": "ping",
	})
	if res.IsError {
		t.Fatalf("direct message by non-executive must work: %s", res.Content[0].Text)
	}
	if n := inboxCount(t, s, "auditor"); n != 1 {
		t.Errorf("auditor should have the direct message, got %d", n)
	}
}

func TestIsExecutiveStore(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"project": "p", "name": "lead", "is_executive": true})
	mustCall(t, s, "register_agent", map[string]any{"project": "p", "name": "dev"})
	cases := []struct {
		agent string
		want  bool
	}{{"lead", true}, {"dev", false}, {"ghost", false}}
	for _, c := range cases {
		got, err := s.store.isExecutive("p", c.agent)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("isExecutive(%q) = %v, want %v", c.agent, got, c.want)
		}
	}
}
