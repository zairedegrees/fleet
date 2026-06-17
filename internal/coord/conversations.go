package coord

import (
	"database/sql"
	"strings"
)

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

// handleStartConversation mints a conversation; if both `to` and `content` are
// supplied it also posts the opening message in the same call (one round-trip).
func handleStartConversation(s *Server, args map[string]any) (toolResult, error) {
	subject := argString(args, "subject")
	if subject == "" {
		return resultError("subject is required"), nil
	}
	project := resolveProject(args)
	from := resolveAgent(args)
	c, err := s.store.createConversation(project, subject, from)
	if err != nil {
		return toolResult{}, err
	}
	var opening *Message
	to := strings.ToLower(argString(args, "to"))
	content := argString(args, "content")
	if to != "" && content != "" {
		opening, err = s.store.sendMessage(
			project, from, to, "notification", subject, content, "{}",
			mapPriority(argString(args, "priority")), nil, &c.ID)
		if err != nil {
			return toolResult{}, err
		}
		c.LastMessageAt = opening.CreatedAt
	}
	return resultText(map[string]any{"conversation": c, "message": opening})
}
