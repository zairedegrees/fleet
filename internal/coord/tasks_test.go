package coord

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/relay"
)

func TestDispatchRoutesByProfileAndCountCapsAtLimit(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"name": "worker1", "project": "p", "profile_slug": "worker"})

	for i := 0; i < 3; i++ {
		mustCall(t, s, "dispatch_task", map[string]any{
			"project": "p", "profile": "worker", "title": fmt.Sprintf("t%d", i), "priority": "P1",
		})
	}

	res := mustCall(t, s, "list_tasks", map[string]any{"project": "p", "profile": "worker", "status": "active", "limit": 2})
	var out struct {
		Count int    `json:"count"`
		Tasks []Task `json:"tasks"`
	}
	decodePayload(t, res, &out)
	if out.Count != 2 {
		t.Errorf("count = %d, want 2 (computed after LIMIT, not the total 3)", out.Count)
	}
	if len(out.Tasks) != 2 {
		t.Errorf("tasks len = %d, want 2", len(out.Tasks))
	}

	// Auto-notify reached the profile's agent but not the dispatcher.
	var toWorker, toDispatcher int
	s.store.reader().QueryRow(`SELECT COUNT(*) FROM messages WHERE project='p' AND to_agent='worker1'`).Scan(&toWorker)
	s.store.reader().QueryRow(`SELECT COUNT(*) FROM messages WHERE to_agent='anonymous'`).Scan(&toDispatcher)
	if toWorker != 3 {
		t.Errorf("worker1 notifications = %d, want 3", toWorker)
	}
	if toDispatcher != 0 {
		t.Errorf("dispatcher notifications = %d, want 0", toDispatcher)
	}
}

// TestDispatchReadsProfileArgKey pins the P0: both dispatch_task and list_tasks
// must read the argument key "profile" (not "profile_slug"). Reading the wrong
// key routes every task to slug "" and makes list_tasks return 0.
func TestDispatchReadsProfileArgKey(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "x", "title": "hi"})

	res := mustCall(t, s, "list_tasks", map[string]any{"project": "p", "profile": "x"})
	var out struct {
		Count int `json:"count"`
	}
	decodePayload(t, res, &out)
	if out.Count != 1 {
		t.Fatalf("dispatch/list by the 'profile' arg key did not route: count=%d", out.Count)
	}
}

func TestListTasksOrdersByPriority(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "w", "title": "low", "priority": "P3"})
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "w", "title": "high", "priority": "P0"})

	res := mustCall(t, s, "list_tasks", map[string]any{"project": "p", "profile": "w"})
	var out struct {
		Tasks []Task `json:"tasks"`
	}
	decodePayload(t, res, &out)
	if len(out.Tasks) != 2 || out.Tasks[0].Priority != "P0" {
		t.Errorf("priority order wrong: %+v", out.Tasks)
	}
}

func TestListTasksTruncatesDescription(t *testing.T) {
	s := New(newTestStore(t))
	long := strings.Repeat("a", 250)
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "w", "title": "x", "description": long})

	res := mustCall(t, s, "list_tasks", map[string]any{"project": "p", "profile": "w"})
	var out struct {
		Tasks []Task `json:"tasks"`
	}
	decodePayload(t, res, &out)
	d := out.Tasks[0].Description
	if !strings.HasSuffix(d, "…") || len([]rune(d)) != 201 {
		t.Errorf("description not truncated to 200+ellipsis: %d runes", len([]rune(d)))
	}
}

// TestCountActiveTasksThroughClient is the Tier-1 proof for the count-after-limit
// path: coord driven through fleet's CountActiveTasks (which sends profile + the
// 500 limit and reads count).
func TestCountActiveTasksThroughClient(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Handler())
	defer srv.Close()
	c := relay.NewClient(srv.URL + "/mcp")

	if err := c.DispatchTask("w", "proj", "task one"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if err := c.DispatchTask("w", "proj", "task two"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	n, err := c.CountActiveTasks("proj", "w")
	if err != nil {
		t.Fatalf("CountActiveTasks: %v", err)
	}
	if n != 2 {
		t.Errorf("CountActiveTasks = %d, want 2", n)
	}
}
