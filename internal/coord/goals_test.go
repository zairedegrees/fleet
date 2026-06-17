package coord

import "testing"

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
