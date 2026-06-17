package coord

import "database/sql"

// createConversation inserts a new conversation. created_at and last_message_at
// start equal; status starts "open".
func (s *Store) createConversation(project, subject, createdBy string) (*Conversation, error) {
	now := nowMicro()
	c := &Conversation{
		ID: newID(), Project: project, Subject: subject, CreatedBy: createdBy,
		CreatedAt: now, LastMessageAt: now, Status: "open",
	}
	err := s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"INSERT INTO conversations (id, project, subject, created_by, created_at, last_message_at, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
			c.ID, c.Project, c.Subject, c.CreatedBy, c.CreatedAt, c.LastMessageAt, c.Status)
		return err
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// handleStartConversation mints a conversation. subject is required. Posting an
// opening message (to+content) is added in a later step.
func handleStartConversation(s *Server, args map[string]any) (toolResult, error) {
	subject := argString(args, "subject")
	if subject == "" {
		return resultError("subject is required"), nil
	}
	c, err := s.store.createConversation(resolveProject(args), subject, resolveAgent(args))
	if err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{"conversation": c, "message": nil})
}
