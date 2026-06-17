package coord

import (
	"database/sql"
	"testing"
)

// dispatchTaskID dispatches a task (optionally under goalID) and returns its id.
func dispatchTaskID(t *testing.T, s *Server, goalID string) string {
	t.Helper()
	args := map[string]any{"project": "p", "as": "lead", "profile": "dev", "title": "T"}
	if goalID != "" {
		args["goal_id"] = goalID
	}
	res := mustCall(t, s, "dispatch_task", args)
	var got struct {
		Task Task `json:"task"`
	}
	decodePayload(t, res, &got)
	return got.Task.ID
}

// setTaskStatus forces a task's status directly (the lifecycle tools require
// specific transitions; for deriving goal progress we just need the end state).
func setTaskStatus(t *testing.T, s *Server, taskID, status string) {
	t.Helper()
	if err := s.store.write(func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE tasks SET status = ? WHERE id = ?", status, taskID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateGoalMintsRow(t *testing.T) {
	s := New(newTestStore(t))
	res := mustCall(t, s, "create_goal", map[string]any{"project": "p", "as": "lead", "title": "Ship auth"})
	var got struct {
		Goal Goal `json:"goal"`
	}
	decodePayload(t, res, &got)
	if got.Goal.ID == "" || got.Goal.Title != "Ship auth" {
		t.Errorf("bad goal: %+v", got.Goal)
	}
	if got.Goal.CreatedBy != "lead" || got.Goal.Status != "open" {
		t.Errorf("bad created_by/status: %+v", got.Goal)
	}
}

func TestCreateGoalRequiresTitle(t *testing.T) {
	s := New(newTestStore(t))
	res := callTool(t, s, "create_goal", map[string]any{"project": "p", "as": "lead"})
	if !res.IsError {
		t.Error("expected error when title missing")
	}
}

func TestDispatchTaskLinksGoal(t *testing.T) {
	s := New(newTestStore(t))
	gr := mustCall(t, s, "create_goal", map[string]any{"project": "p", "as": "lead", "title": "G"})
	var g struct {
		Goal Goal `json:"goal"`
	}
	decodePayload(t, gr, &g)
	res := mustCall(t, s, "dispatch_task", map[string]any{
		"project": "p", "as": "lead", "profile": "dev", "title": "T", "goal_id": g.Goal.ID,
	})
	var got struct {
		Task Task `json:"task"`
	}
	decodePayload(t, res, &got)
	if got.Task.GoalID == nil || *got.Task.GoalID != g.Goal.ID {
		t.Errorf("task should carry goal_id, got %+v", got.Task)
	}
}

func TestDispatchTaskRejectsUnknownGoal(t *testing.T) {
	s := New(newTestStore(t))
	res := callTool(t, s, "dispatch_task", map[string]any{
		"project": "p", "as": "lead", "profile": "dev", "title": "T", "goal_id": "nope",
	})
	if !res.IsError {
		t.Fatal("expected error for unknown goal_id")
	}
}

func TestDispatchTaskNoGoalUnchanged(t *testing.T) {
	s := New(newTestStore(t))
	res := mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "as": "lead", "profile": "dev", "title": "T"})
	var got struct {
		Task Task `json:"task"`
	}
	decodePayload(t, res, &got)
	if got.Task.GoalID != nil {
		t.Errorf("task without goal_id should have nil GoalID, got %v", *got.Task.GoalID)
	}
}

func TestGetGoalProgress(t *testing.T) {
	s := New(newTestStore(t))
	gr := mustCall(t, s, "create_goal", map[string]any{"project": "p", "as": "lead", "title": "G"})
	var g struct {
		Goal Goal `json:"goal"`
	}
	decodePayload(t, gr, &g)
	gid := g.Goal.ID
	t1 := dispatchTaskID(t, s, gid)
	t2 := dispatchTaskID(t, s, gid)
	t3 := dispatchTaskID(t, s, gid)
	dispatchTaskID(t, s, gid) // stays pending
	setTaskStatus(t, s, t1, "done")
	setTaskStatus(t, s, t2, "blocked")
	setTaskStatus(t, s, t3, "cancelled")

	res := mustCall(t, s, "get_goal", map[string]any{"project": "p", "goal_id": gid})
	var got struct {
		Goal     Goal `json:"goal"`
		Progress struct {
			Total      int `json:"total"`
			Done       int `json:"done"`
			Blocked    int `json:"blocked"`
			InProgress int `json:"in_progress"`
		} `json:"progress"`
	}
	decodePayload(t, res, &got)
	// 4 dispatched, 1 cancelled excluded -> total 3; done 1; blocked 1; in_progress 1 (pending).
	if got.Progress.Total != 3 || got.Progress.Done != 1 || got.Progress.Blocked != 1 || got.Progress.InProgress != 1 {
		t.Errorf("bad progress: %+v", got.Progress)
	}
}

func TestGetGoalEmptyProgress(t *testing.T) {
	s := New(newTestStore(t))
	gr := mustCall(t, s, "create_goal", map[string]any{"project": "p", "as": "lead", "title": "G"})
	var g struct {
		Goal Goal `json:"goal"`
	}
	decodePayload(t, gr, &g)
	res := mustCall(t, s, "get_goal", map[string]any{"project": "p", "goal_id": g.Goal.ID})
	var got struct {
		Progress struct {
			Total int `json:"total"`
		} `json:"progress"`
	}
	decodePayload(t, res, &got)
	if got.Progress.Total != 0 {
		t.Errorf("empty goal should have total 0, got %d", got.Progress.Total)
	}
}

func TestGetGoalNotFound(t *testing.T) {
	s := New(newTestStore(t))
	res := callTool(t, s, "get_goal", map[string]any{"project": "p", "goal_id": "nope"})
	if !res.IsError {
		t.Error("expected not-found error")
	}
}
