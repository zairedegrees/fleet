package coord

import (
	"strings"
	"testing"
)

// dispatchOne dispatches a task and returns its id.
func dispatchOne(t *testing.T, s *Server, project, profile, title string) string {
	t.Helper()
	res := mustCall(t, s, "dispatch_task", map[string]any{"project": project, "profile": profile, "title": title})
	var d struct {
		Task Task `json:"task"`
	}
	decodePayload(t, res, &d)
	return d.Task.ID
}

func TestTaskLifecyclePending_To_Done(t *testing.T) {
	s := New(newTestStore(t))
	id := dispatchOne(t, s, "p", "w", "do the thing")

	var claimed Task
	decodePayload(t, mustCall(t, s, "claim_task", map[string]any{"as": "worker1", "project": "p", "task_id": id}), &claimed)
	if claimed.Status != "accepted" || claimed.AssignedTo == nil || *claimed.AssignedTo != "worker1" || claimed.AcceptedAt == nil {
		t.Fatalf("claim: %+v", claimed)
	}

	var started Task
	decodePayload(t, mustCall(t, s, "start_task", map[string]any{"as": "worker1", "project": "p", "task_id": id}), &started)
	if started.Status != "in-progress" || started.StartedAt == nil {
		t.Fatalf("start: %+v", started)
	}

	var done Task
	decodePayload(t, mustCall(t, s, "complete_task", map[string]any{"as": "worker1", "project": "p", "task_id": id, "result": "shipped"}), &done)
	if done.Status != "done" || done.Result == nil || *done.Result != "shipped" || done.CompletedAt == nil {
		t.Fatalf("complete: %+v", done)
	}
}

func TestInvalidTransitionIsError(t *testing.T) {
	s := New(newTestStore(t))
	id := dispatchOne(t, s, "p", "w", "do")

	// pending -> blocked is not allowed (blocked only from in-progress).
	res := callTool(t, s, "block_task", map[string]any{"as": "a", "project": "p", "task_id": id})
	if !res.IsError {
		t.Fatal("expected invalid-transition error result")
	}
	if !strings.Contains(res.Content[0].Text, "invalid transition") {
		t.Errorf("error message = %q", res.Content[0].Text)
	}
}

func TestUserBypassesTransitionValidation(t *testing.T) {
	s := New(newTestStore(t))
	id := dispatchOne(t, s, "p", "w", "do")

	// as="user" force-moves pending -> blocked despite the state machine.
	var bt Task
	decodePayload(t, mustCall(t, s, "block_task", map[string]any{"as": "user", "project": "p", "task_id": id, "reason": "manual"}), &bt)
	if bt.Status != "blocked" || bt.BlockedReason == nil || *bt.BlockedReason != "manual" {
		t.Fatalf("user force-block: %+v", bt)
	}
}

func TestBlockThenResumeViaGetTask(t *testing.T) {
	s := New(newTestStore(t))
	id := dispatchOne(t, s, "p", "w", "do")
	mustCall(t, s, "start_task", map[string]any{"as": "a", "project": "p", "task_id": id})
	mustCall(t, s, "block_task", map[string]any{"as": "a", "project": "p", "task_id": id, "reason": "waiting on X"})

	var blocked Task
	decodePayload(t, mustCall(t, s, "get_task", map[string]any{"project": "p", "task_id": id}), &blocked)
	if blocked.Status != "blocked" || blocked.BlockedReason == nil || *blocked.BlockedReason != "waiting on X" {
		t.Fatalf("get_task after block: %+v", blocked)
	}

	mustCall(t, s, "start_task", map[string]any{"as": "a", "project": "p", "task_id": id})
	var resumed Task
	decodePayload(t, mustCall(t, s, "get_task", map[string]any{"project": "p", "task_id": id}), &resumed)
	if resumed.Status != "in-progress" {
		t.Errorf("resume from blocked failed: %s", resumed.Status)
	}
}

func TestTaskIDPrefixResolution(t *testing.T) {
	s := New(newTestStore(t))
	id := dispatchOne(t, s, "p", "w", "do")

	var claimed Task
	decodePayload(t, mustCall(t, s, "claim_task", map[string]any{"as": "a", "project": "p", "task_id": id[:8]}), &claimed)
	if claimed.ID != id || claimed.Status != "accepted" {
		t.Fatalf("prefix resolution: got %s/%s", claimed.ID, claimed.Status)
	}
}

func TestTransitionMissingTaskIsError(t *testing.T) {
	s := New(newTestStore(t))
	res := callTool(t, s, "claim_task", map[string]any{"as": "a", "project": "p", "task_id": "00000000-0000-4000-8000-000000000000"})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "task not found") {
		t.Errorf("expected task-not-found error, got isErr=%v %q", res.IsError, res.Content[0].Text)
	}
}
