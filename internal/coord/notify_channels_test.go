package coord

import "testing"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(newTestStore(t))
}

func seedActiveAgent(t *testing.T, s *Server, project, name, profile string) {
	t.Helper()
	mustCall(t, s, "register_agent", map[string]any{"name": name, "project": project, "profile_slug": profile})
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

// TestAgentsWithPendingTasks verifies the sweep returns recipients of unread
// (queued) task-type messages that have a registered notify channel — excluding
// read, non-task, and channel-less recipients.
func TestAgentsWithPendingTasks(t *testing.T) {
	s := newTestServer(t)
	// Seed two active agents on profile "dev" (mirror the existing seeding helper).
	seedActiveAgent(t, s, "acme", "worker", "dev")
	seedActiveAgent(t, s, "acme", "lead", "dev")
	_ = s.store.registerNotifyChannel("acme", "worker", "tmux:fleet-acme-worker")

	// lead dispatches a task on profile dev → a queued type='task' delivery for worker.
	if _, _, err := s.store.dispatchTask("acme", "dev", "lead", "ship it", "", "P2", ""); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	got, err := s.AgentsWithPendingTasks()
	if err != nil {
		t.Fatalf("query err: %v", err)
	}
	if len(got) != 1 || got[0].Agent != "worker" || got[0].Project != "acme" {
		t.Fatalf("want [acme/worker], got %+v", got)
	}
}

func TestAgentsWithPendingTasksExcludesChannelless(t *testing.T) {
	s := newTestServer(t)
	seedActiveAgent(t, s, "acme", "worker", "dev")
	seedActiveAgent(t, s, "acme", "lead", "dev")
	// No notify channel for worker.
	if _, _, err := s.store.dispatchTask("acme", "dev", "lead", "ship it", "", "P2", ""); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got, err := s.AgentsWithPendingTasks()
	if err != nil {
		t.Fatalf("query err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("channel-less recipient must be excluded, got %+v", got)
	}
}

// A task the agent polled (delivery surfaced via get_inbox) but never mark_read'd
// is still unread — the sweep must keep waking it. This mirrors getInbox's
// pending definition (state IN ('queued','surfaced')); a queued-only filter would
// silently drop idle agents that fetched but never acted, the exact case the
// waker exists for.
func TestAgentsWithPendingTasksIncludesSurfaced(t *testing.T) {
	s := newTestServer(t)
	seedActiveAgent(t, s, "acme", "worker", "dev")
	seedActiveAgent(t, s, "acme", "lead", "dev")
	_ = s.store.registerNotifyChannel("acme", "worker", "tmux:fleet-acme-worker")
	if _, _, err := s.store.dispatchTask("acme", "dev", "lead", "ship it", "", "P2", ""); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// Polling the inbox surfaces the queued delivery (queued → surfaced).
	if _, err := s.store.getInbox("acme", "worker", false, 50); err != nil {
		t.Fatalf("get_inbox: %v", err)
	}

	got, err := s.AgentsWithPendingTasks()
	if err != nil {
		t.Fatalf("query err: %v", err)
	}
	if len(got) != 1 || got[0].Agent != "worker" || got[0].Project != "acme" {
		t.Fatalf("surfaced (unread) task must remain a wake candidate, got %+v", got)
	}
}

// Once the agent mark_read's the task (delivery → acknowledged) it is no longer
// unread and must drop out of the sweep.
func TestAgentsWithPendingTasksExcludesAcknowledged(t *testing.T) {
	s := newTestServer(t)
	seedActiveAgent(t, s, "acme", "worker", "dev")
	seedActiveAgent(t, s, "acme", "lead", "dev")
	_ = s.store.registerNotifyChannel("acme", "worker", "tmux:fleet-acme-worker")
	if _, _, err := s.store.dispatchTask("acme", "dev", "lead", "ship it", "", "P2", ""); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// The dispatch notification is the only task-typed message to worker; mark it
	// read so its delivery moves to 'acknowledged'.
	inbox, err := s.store.getInbox("acme", "worker", false, 50)
	if err != nil {
		t.Fatalf("get_inbox: %v", err)
	}
	var ids []string
	for _, m := range inbox {
		if m.Type == "task" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		t.Fatalf("expected a task-typed dispatch message for worker, inbox=%+v", inbox)
	}
	if _, err := s.store.markRead(ids, "worker", "acme"); err != nil {
		t.Fatalf("mark_read: %v", err)
	}

	got, err := s.AgentsWithPendingTasks()
	if err != nil {
		t.Fatalf("query err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("acknowledged (read) task must be excluded, got %+v", got)
	}
}

// project is the isolation boundary: a pending task in one project must not leak
// into another's sweep, and each candidate carries its own project. The channel
// join is keyed on (agent_name, project), so a same-named agent in a project
// without a channel must also be excluded.
func TestAgentsWithPendingTasksIsolatesProjects(t *testing.T) {
	s := newTestServer(t)
	// acme/worker: pending task + channel → candidate.
	seedActiveAgent(t, s, "acme", "worker", "dev")
	seedActiveAgent(t, s, "acme", "lead", "dev")
	_ = s.store.registerNotifyChannel("acme", "worker", "tmux:fleet-acme-worker")
	if _, _, err := s.store.dispatchTask("acme", "dev", "lead", "a", "", "P2", ""); err != nil {
		t.Fatalf("dispatch acme: %v", err)
	}
	// beta/worker: pending task but NO channel in beta → excluded.
	seedActiveAgent(t, s, "beta", "worker", "dev")
	seedActiveAgent(t, s, "beta", "lead", "dev")
	if _, _, err := s.store.dispatchTask("beta", "dev", "lead", "b", "", "P2", ""); err != nil {
		t.Fatalf("dispatch beta: %v", err)
	}

	got, err := s.AgentsWithPendingTasks()
	if err != nil {
		t.Fatalf("query err: %v", err)
	}
	if len(got) != 1 || got[0].Project != "acme" || got[0].Agent != "worker" {
		t.Fatalf("want only [acme/worker]; beta has no channel and must not leak, got %+v", got)
	}
}
