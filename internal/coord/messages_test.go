package coord

import (
	"strings"
	"testing"
)

func TestSendThenInboxSurfaces(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "send_message", map[string]any{
		"as": "alice", "to": "Bob", "project": "p", "subject": "hi", "content": "hello bob",
	})

	// First read: message present, reported as queued (pre-surface state).
	var out struct {
		Count    int              `json:"count"`
		Messages []map[string]any `json:"messages"`
	}
	decodePayload(t, mustCall(t, s, "get_inbox", map[string]any{"as": "bob", "project": "p"}), &out)
	if out.Count != 1 {
		t.Fatalf("inbox count = %d, want 1", out.Count)
	}
	m := out.Messages[0]
	if m["from"] != "alice" || m["subject"] != "hi" {
		t.Errorf("unexpected message: %v", m)
	}
	if m["delivery_state"] != "queued" {
		t.Errorf("first read delivery_state = %v, want queued", m["delivery_state"])
	}

	// The delivery was surfaced as a side effect: unread_only no longer returns it.
	var unread struct {
		Count int `json:"count"`
	}
	decodePayload(t, mustCall(t, s, "get_inbox", map[string]any{"as": "bob", "project": "p", "unread_only": true}), &unread)
	if unread.Count != 0 {
		t.Errorf("after surfacing, unread count = %d, want 0", unread.Count)
	}

	// But unread_only=false still shows it, now surfaced.
	var all struct {
		Count    int              `json:"count"`
		Messages []map[string]any `json:"messages"`
	}
	decodePayload(t, mustCall(t, s, "get_inbox", map[string]any{"as": "bob", "project": "p", "unread_only": false}), &all)
	if all.Count != 1 || all.Messages[0]["delivery_state"] != "surfaced" {
		t.Errorf("surfaced inbox: %+v", all)
	}
}

func TestMarkReadCountsOnlyNewAndAcks(t *testing.T) {
	s := New(newTestStore(t))
	var msg Message
	decodePayload(t, mustCall(t, s, "send_message", map[string]any{"as": "a", "to": "b", "project": "p", "content": "x"}), &msg)

	var first struct {
		MarkedRead int `json:"marked_read"`
	}
	decodePayload(t, mustCall(t, s, "mark_read", map[string]any{"as": "b", "project": "p", "message_ids": []any{msg.ID}}), &first)
	if first.MarkedRead != 1 {
		t.Errorf("first mark_read = %d, want 1", first.MarkedRead)
	}

	var second struct {
		MarkedRead int `json:"marked_read"`
	}
	decodePayload(t, mustCall(t, s, "mark_read", map[string]any{"as": "b", "project": "p", "message_ids": []any{msg.ID}}), &second)
	if second.MarkedRead != 0 {
		t.Errorf("re-mark_read = %d, want 0 (only new receipts count)", second.MarkedRead)
	}

	// Acknowledged delivery drops out of the inbox entirely.
	var inbox struct {
		Count int `json:"count"`
	}
	decodePayload(t, mustCall(t, s, "get_inbox", map[string]any{"as": "b", "project": "p", "unread_only": false}), &inbox)
	if inbox.Count != 0 {
		t.Errorf("acknowledged message still in inbox: count %d", inbox.Count)
	}
}

func TestInboxOrdersByPriorityAndTruncates(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "send_message", map[string]any{"as": "a", "to": "b", "project": "p", "subject": "low", "content": "l", "priority": "P2"})
	mustCall(t, s, "send_message", map[string]any{"as": "a", "to": "b", "project": "p", "subject": "high", "content": strings.Repeat("z", 350), "priority": "interrupt"})

	var out struct {
		Messages []map[string]any `json:"messages"`
	}
	decodePayload(t, mustCall(t, s, "get_inbox", map[string]any{"as": "b", "project": "p"}), &out)
	if len(out.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(out.Messages))
	}
	// priority ASC: the "interrupt"->P0 message sorts first.
	if out.Messages[0]["subject"] != "high" || out.Messages[0]["priority"] != "P0" {
		t.Errorf("priority order/mapping wrong: %v", out.Messages[0])
	}
	c := out.Messages[0]["content"].(string)
	if !strings.HasSuffix(c, "...") || len(c) != 303 {
		t.Errorf("content not truncated to 300+...: len %d", len(c))
	}
}

func TestDispatchAutoNotifyVisibleInInbox(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"name": "worker1", "project": "p", "profile_slug": "worker"})
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "worker", "title": "do", "priority": "P1"})

	var out struct {
		Count    int              `json:"count"`
		Messages []map[string]any `json:"messages"`
	}
	decodePayload(t, mustCall(t, s, "get_inbox", map[string]any{"as": "worker1", "project": "p"}), &out)
	if out.Count != 1 || out.Messages[0]["type"] != "task" {
		t.Fatalf("dispatch auto-notify not visible in inbox: %+v", out)
	}
}

func TestBroadcastDeliversToAllExceptSender(t *testing.T) {
	s := New(newTestStore(t))
	for _, n := range []string{"a", "b", "c"} {
		mustCall(t, s, "register_agent", map[string]any{"name": n, "project": "p"})
	}
	mustCall(t, s, "send_message", map[string]any{"as": "a", "to": "*", "project": "p", "content": "all hands"})

	for _, who := range []string{"b", "c"} {
		var o struct {
			Count int `json:"count"`
		}
		decodePayload(t, mustCall(t, s, "get_inbox", map[string]any{"as": who, "project": "p"}), &o)
		if o.Count != 1 {
			t.Errorf("%s broadcast inbox count = %d, want 1", who, o.Count)
		}
	}
	var sender struct {
		Count int `json:"count"`
	}
	decodePayload(t, mustCall(t, s, "get_inbox", map[string]any{"as": "a", "project": "p"}), &sender)
	if sender.Count != 0 {
		t.Errorf("sender received own broadcast: count %d", sender.Count)
	}
}

func TestSendAndMarkReadValidation(t *testing.T) {
	s := New(newTestStore(t))
	if res := callTool(t, s, "send_message", map[string]any{"as": "a", "project": "p", "content": "x"}); !res.IsError || !strings.Contains(res.Content[0].Text, "to is required") {
		t.Errorf("send without to: isErr=%v %q", res.IsError, res.Content[0].Text)
	}
	if res := callTool(t, s, "mark_read", map[string]any{"as": "b", "project": "p"}); !res.IsError || !strings.Contains(res.Content[0].Text, "message_ids or conversation_id") {
		t.Errorf("mark_read without ids: isErr=%v %q", res.IsError, res.Content[0].Text)
	}
}
