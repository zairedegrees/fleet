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
