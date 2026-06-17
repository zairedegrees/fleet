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

// getConversationMeta returns the conversation row, or nil if absent in project.
func (s *Store) getConversationMeta(project, id string) (*Conversation, error) {
	var c Conversation
	err := s.reader().QueryRow(
		"SELECT id, project, subject, created_by, created_at, last_message_at, status FROM conversations WHERE id = ? AND project = ?",
		id, project).Scan(&c.ID, &c.Project, &c.Subject, &c.CreatedBy, &c.CreatedAt, &c.LastMessageAt, &c.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// listConversationMessages returns up to limit messages of a conversation,
// newest-first internally (optionally older than the `before` created_at
// cursor), then rendered chronologically (ascending). It fetches one extra row
// to report has_more. Content is truncated to 300 chars unless fullContent.
func (s *Store) listConversationMessages(project, convID, before string, limit int, fullContent bool) ([]map[string]any, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	q := `SELECT id, from_agent, to_agent, type, subject, content, created_at, priority
	      FROM messages WHERE project = ? AND conversation_id = ?`
	a := []any{project, convID}
	if before != "" {
		q += " AND created_at < ?"
		a = append(a, before)
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	a = append(a, limit+1)

	rows, err := s.reader().Query(q, a...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	type row struct{ id, from, to, typ, subject, content, createdAt, priority string }
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.from, &r.to, &r.typ, &r.subject, &r.content, &r.createdAt, &r.priority); err != nil {
			return nil, false, err
		}
		rs = append(rs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(rs) > limit
	if hasMore {
		rs = rs[:limit]
	}
	out := make([]map[string]any, 0, len(rs))
	for i := len(rs) - 1; i >= 0; i-- { // reverse to chronological ASC
		r := rs[i]
		content := r.content
		if !fullContent && len(content) > 300 {
			content = content[:300] + "..."
		}
		out = append(out, map[string]any{
			"id": r.id, "from": r.from, "to": r.to, "type": r.typ,
			"subject": r.subject, "content": content, "created_at": r.createdAt, "priority": r.priority,
		})
	}
	return out, hasMore, nil
}

func handleGetConversation(s *Server, args map[string]any) (toolResult, error) {
	project := resolveProject(args)
	id := argString(args, "conversation_id")
	if id == "" {
		return resultError("conversation_id is required"), nil
	}
	meta, err := s.store.getConversationMeta(project, id)
	if err != nil {
		return toolResult{}, err
	}
	if meta == nil {
		return resultError("conversation not found"), nil
	}
	msgs, hasMore, err := s.store.listConversationMessages(
		project, id, argString(args, "before"), argInt(args, "limit", 20), argBool(args, "full_content", false))
	if err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{
		"conversation": meta, "messages": msgs, "count": len(msgs), "has_more": hasMore,
	})
}
