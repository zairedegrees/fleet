package coord

import (
	"database/sql"
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
// must read the argument key "profile" (not "profile_slug"). The assertions are
// isolated so a regression to "profile_slug" actually fails — reading the wrong
// key on dispatch writes profile_slug="" (caught by the column check), and on
// list drops the WHERE filter (caught by the two-profile count check).
func TestDispatchReadsProfileArgKey(t *testing.T) {
	s := New(newTestStore(t))

	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "x", "title": "hi"})

	// dispatch wrote profile_slug from the "profile" arg — not "".
	var slug string
	if err := s.store.reader().QueryRow(`SELECT profile_slug FROM tasks WHERE project='p' LIMIT 1`).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug != "x" {
		t.Fatalf("dispatch read the wrong arg key: task.profile_slug = %q, want x", slug)
	}

	// A second task on profile "y": listing "x" must return exactly 1 (fails if
	// list_tasks reads the wrong key and drops the profile filter, returning 2).
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "y", "title": "yo"})
	res := mustCall(t, s, "list_tasks", map[string]any{"project": "p", "profile": "x"})
	var out struct {
		Count int `json:"count"`
	}
	decodePayload(t, res, &out)
	if out.Count != 1 {
		t.Fatalf("list_tasks filter by 'profile' wrong: count=%d, want 1", out.Count)
	}
}

// TestDispatchExcludesDispatcherFromNotify makes the `n == dispatchedBy` skip
// load-bearing: two agents share the profile and one IS the dispatcher.
func TestDispatchExcludesDispatcherFromNotify(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"name": "lead", "project": "p", "profile_slug": "worker"})
	mustCall(t, s, "register_agent", map[string]any{"name": "worker1", "project": "p", "profile_slug": "worker"})

	mustCall(t, s, "dispatch_task", map[string]any{"as": "lead", "project": "p", "profile": "worker", "title": "task"})

	var toLead, toWorker int
	s.store.reader().QueryRow(`SELECT COUNT(*) FROM messages WHERE to_agent='lead'`).Scan(&toLead)
	s.store.reader().QueryRow(`SELECT COUNT(*) FROM messages WHERE to_agent='worker1'`).Scan(&toWorker)
	if toLead != 0 {
		t.Errorf("dispatcher 'lead' was self-notified (%d); exclusion branch is dead", toLead)
	}
	if toWorker != 1 {
		t.Errorf("worker1 notify = %d, want 1", toWorker)
	}
}

// TestAutoNotifyMessageContent pins the notify message wire (type/subject/
// content/metadata/priority/ttl) against wrai.th — the agent-skill-visible part.
func TestAutoNotifyMessageContent(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"name": "worker1", "project": "p", "profile_slug": "worker"})
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "worker", "title": "ship it", "priority": "P1"})

	var typ, subject, content, metadata, priority string
	var ttl int
	if err := s.store.reader().QueryRow(
		`SELECT type, subject, content, metadata, priority, ttl_seconds FROM messages WHERE to_agent='worker1'`).
		Scan(&typ, &subject, &content, &metadata, &priority, &ttl); err != nil {
		t.Fatal(err)
	}
	if typ != "task" {
		t.Errorf("type = %q, want task", typ)
	}
	if subject != "New task: ship it" {
		t.Errorf("subject = %q", subject)
	}
	if !strings.HasPrefix(content, "[P1] ship it") {
		t.Errorf("content prefix = %q", content)
	}
	if !strings.Contains(content, "Profile: worker") || !strings.Contains(content, "Dispatched by: anonymous") {
		t.Errorf("content missing fields: %q", content)
	}
	if priority != "P2" {
		t.Errorf("notify priority = %q, want P2", priority)
	}
	if ttl != 14400 {
		t.Errorf("ttl_seconds = %d, want 14400", ttl)
	}
	if !strings.Contains(metadata, `"task_id"`) {
		t.Errorf("metadata missing task_id: %q", metadata)
	}
}

// TestListTasksActiveExcludesTerminal proves the status='active' filter excludes
// done/cancelled — the exact filter the live CountActiveTasks consumer sends. A
// done row is inserted directly since the transition handlers are a later wave.
func TestListTasksActiveExcludesTerminal(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "w", "title": "pending one"})
	if err := s.store.write(func(tx *sql.Tx) error {
		_, e := tx.Exec(
			`INSERT INTO tasks (id, profile_slug, dispatched_by, title, status, priority, project, dispatched_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			newID(), "w", "x", "done one", "done", "P2", "p", nowMicro())
		return e
	}); err != nil {
		t.Fatal(err)
	}

	active := mustCall(t, s, "list_tasks", map[string]any{"project": "p", "profile": "w", "status": "active"})
	var ao struct {
		Count int `json:"count"`
	}
	decodePayload(t, active, &ao)
	if ao.Count != 1 {
		t.Errorf("active count = %d, want 1 (done excluded)", ao.Count)
	}

	all := mustCall(t, s, "list_tasks", map[string]any{"project": "p", "profile": "w"})
	var allo struct {
		Count int `json:"count"`
	}
	decodePayload(t, all, &allo)
	if allo.Count != 2 {
		t.Errorf("unfiltered count = %d, want 2 (done included)", allo.Count)
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
