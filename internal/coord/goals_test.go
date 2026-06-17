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
