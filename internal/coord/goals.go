package coord

import "database/sql"

// createGoal inserts a new goal (status "open").
func (s *Store) createGoal(project, title, description, createdBy string) (*Goal, error) {
	g := &Goal{
		ID: newID(), Project: project, Title: title, Description: description,
		CreatedBy: createdBy, CreatedAt: nowMicro(), Status: "open",
	}
	err := s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"INSERT INTO goals (id, project, title, description, created_by, created_at, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
			g.ID, g.Project, g.Title, g.Description, g.CreatedBy, g.CreatedAt, g.Status)
		return err
	})
	if err != nil {
		return nil, err
	}
	return g, nil
}

func handleCreateGoal(s *Server, args map[string]any) (toolResult, error) {
	title := argString(args, "title")
	if title == "" {
		return resultError("title is required"), nil
	}
	g, err := s.store.createGoal(resolveProject(args), title, argString(args, "description"), resolveAgent(args))
	if err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{"goal": g})
}

// getGoalMeta returns the goal row, or nil if absent in project.
func (s *Store) getGoalMeta(project, id string) (*Goal, error) {
	var g Goal
	err := s.reader().QueryRow(
		"SELECT id, project, title, description, created_by, created_at, status FROM goals WHERE id = ? AND project = ?",
		id, project).Scan(&g.ID, &g.Project, &g.Title, &g.Description, &g.CreatedBy, &g.CreatedAt, &g.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// goalProgress derives task counts for a goal: total (non-cancelled), done and
// blocked. Archived tasks are excluded. in_progress is total-done-blocked.
func (s *Store) goalProgress(project, goalID string) (total, done, blocked int, err error) {
	err = s.reader().QueryRow(`
SELECT
  COALESCE(SUM(CASE WHEN status != 'cancelled' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END), 0)
FROM tasks WHERE goal_id = ? AND project = ? AND archived_at IS NULL`,
		goalID, project).Scan(&total, &done, &blocked)
	return total, done, blocked, err
}

func handleGetGoal(s *Server, args map[string]any) (toolResult, error) {
	project := resolveProject(args)
	id := argString(args, "goal_id")
	if id == "" {
		return resultError("goal_id is required"), nil
	}
	meta, err := s.store.getGoalMeta(project, id)
	if err != nil {
		return toolResult{}, err
	}
	if meta == nil {
		return resultError("goal not found"), nil
	}
	total, done, blocked, err := s.store.goalProgress(project, id)
	if err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{
		"goal": meta,
		"progress": map[string]any{
			"total": total, "done": done, "blocked": blocked, "in_progress": total - done - blocked,
		},
	})
}
