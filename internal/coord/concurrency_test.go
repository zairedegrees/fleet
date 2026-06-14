package coord

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestConcurrentDispatchAndInbox stresses the single-writer discipline under the
// real handlers: many dispatches (each inserts a task + fans out a delivery to
// every worker) run concurrently with inbox polls (which surface deliveries via
// the writer). Asserts no "database is locked" surfaces and the final state is
// consistent.
func TestConcurrentDispatchAndInbox(t *testing.T) {
	s := New(newTestStore(t))
	const nAgents = 8
	for i := 0; i < nAgents; i++ {
		mustCall(t, s, "register_agent", map[string]any{"name": fmt.Sprintf("w%d", i), "project": "p", "profile_slug": "worker"})
	}

	const nTasks = 40
	var wg sync.WaitGroup
	errs := make(chan error, nTasks+nAgents)

	for i := 0; i < nTasks; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res := callTool(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "worker", "title": fmt.Sprintf("t%d", i), "priority": "P1"})
			if res.IsError {
				errs <- fmt.Errorf("dispatch %d: %s", i, res.Content[0].Text)
			}
		}(i)
	}
	for i := 0; i < nAgents; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				res := callTool(t, s, "get_inbox", map[string]any{"as": fmt.Sprintf("w%d", i), "project": "p", "limit": 50})
				if res.IsError {
					errs <- fmt.Errorf("inbox w%d: %s", i, res.Content[0].Text)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent op failed: %v", err)
	}

	var out struct {
		Count int `json:"count"`
	}
	decodePayload(t, mustCall(t, s, "list_tasks", map[string]any{"project": "p", "profile": "worker", "status": "active", "limit": 100}), &out)
	if out.Count != nTasks {
		t.Errorf("final active task count = %d, want %d", out.Count, nTasks)
	}
}

// TestConcurrentClaimRace proves the writer-Mutex spans the whole transition
// transaction: when N agents race to claim the same pending task, exactly one
// wins (the rest see "accepted" and fail the transition) — no two observe
// "pending" and double-assign.
func TestConcurrentClaimRace(t *testing.T) {
	s := New(newTestStore(t))
	id := dispatchOne(t, s, "p", "w", "race")

	const n = 8
	var wg sync.WaitGroup
	var successes int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res := callTool(t, s, "claim_task", map[string]any{"as": fmt.Sprintf("a%d", i), "project": "p", "task_id": id})
			if !res.IsError {
				atomic.AddInt32(&successes, 1)
			}
		}(i)
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("claim race: %d agents succeeded, want exactly 1", successes)
	}
	// And the task is exactly once-claimed (accepted).
	var task Task
	decodePayload(t, mustCall(t, s, "get_task", map[string]any{"project": "p", "task_id": id}), &task)
	if task.Status != "accepted" {
		t.Errorf("task status = %q, want accepted", task.Status)
	}
}
