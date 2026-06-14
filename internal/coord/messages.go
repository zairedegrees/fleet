package coord

import (
	"database/sql"
	"strings"

	"github.com/zairedegrees/fleet/internal/coord/normalize"
)

// mapPriority normalizes message priority aliases to P0-P3 (default P2), like
// wrai.th. Unlike task priority (raw P0-P3), message priority is alias-mapped.
func mapPriority(p string) string {
	switch strings.ToLower(p) {
	case "interrupt", "p0":
		return "P0"
	case "steering", "p1":
		return "P1"
	case "advisory", "p2", "":
		return "P2"
	case "info", "p3":
		return "P3"
	default:
		return "P2"
	}
}

// createDeliveryTx queues one delivery per recipient. coord is delivery-only:
// every inbox-visible message needs a delivery (there is no legacy fallback).
func createDeliveryTx(tx *sql.Tx, messageID, project string, recipients []string) error {
	now := nowMicro()
	for i, agent := range recipients {
		if _, err := tx.Exec(
			"INSERT INTO deliveries (id, message_id, to_agent, state, sequence_number, created_at, project) VALUES (?, ?, ?, 'queued', ?, ?, ?)",
			newID(), messageID, agent, i, now, project); err != nil {
			return err
		}
	}
	return nil
}

// resolveRecipientsTx expands a destination: "*" broadcasts to every non-sender
// agent in the project, anything else is a direct single recipient. (Conversation
// fan-out is out of scope for this wave.)
func resolveRecipientsTx(tx *sql.Tx, project, to, from string) ([]string, error) {
	if to != "*" {
		return []string{to}, nil
	}
	rows, err := tx.Query("SELECT name FROM agents WHERE project = ? AND status IN ('active', 'sleeping', 'inactive')", project)
	if err != nil {
		return nil, err
	}
	var recipients []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		if n != from {
			recipients = append(recipients, n)
		}
	}
	rows.Close()
	return recipients, rows.Err()
}

func (s *Store) sendMessage(project, from, to, msgType, subject, content, metadata, priority string, replyTo, conversationID *string) (*Message, error) {
	if metadata == "" {
		metadata = "{}"
	}
	msg := &Message{
		ID: newID(), From: from, To: to, ReplyTo: replyTo, Type: msgType, Subject: subject,
		Content: normalize.JSONKeys(content), Metadata: normalize.JSONKeys(metadata),
		CreatedAt: nowMicro(), ConversationID: conversationID, Project: project,
		Priority: priority, TTLSeconds: 14400,
	}
	err := s.write(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			"INSERT INTO messages (id, from_agent, to_agent, reply_to, type, subject, content, metadata, created_at, conversation_id, project, priority, ttl_seconds) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			msg.ID, msg.From, msg.To, msg.ReplyTo, msg.Type, msg.Subject, msg.Content, msg.Metadata, msg.CreatedAt, msg.ConversationID, msg.Project, msg.Priority, msg.TTLSeconds); err != nil {
			return err
		}
		recipients, err := resolveRecipientsTx(tx, project, to, from)
		if err != nil {
			return err
		}
		return createDeliveryTx(tx, msg.ID, project, recipients)
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// getInbox returns an agent's pending messages (delivery state queued/surfaced)
// as full Message objects with the delivery id/state attached, ordered by
// priority then recency, and surfaces the queued ones as a side effect. The
// returned messages carry the pre-surface delivery state.
func (s *Store) getInbox(project, agent string, unreadOnly bool, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `SELECT m.id, m.from_agent, m.to_agent, m.reply_to, m.type, m.subject, m.content, m.metadata,
	                 m.created_at, m.read_at, m.conversation_id, m.project, m.task_id, m.priority, m.ttl_seconds, m.expired_at,
	                 d.id, d.state
	          FROM deliveries d JOIN messages m ON d.message_id = m.id
	          WHERE d.project = ? AND d.to_agent = ? AND d.state IN ('queued', 'surfaced') AND m.expired_at IS NULL`
	if unreadOnly {
		query += " AND d.state = 'queued'"
	}
	query += " ORDER BY m.priority ASC, m.created_at DESC LIMIT ?"

	rows, err := s.reader().Query(query, project, agent, limit)
	if err != nil {
		return nil, err
	}
	msgs := []Message{}
	var queuedIDs []string
	for rows.Next() {
		var m Message
		var deliveryID, deliveryState string
		if err := rows.Scan(&m.ID, &m.From, &m.To, &m.ReplyTo, &m.Type, &m.Subject, &m.Content, &m.Metadata,
			&m.CreatedAt, &m.ReadAt, &m.ConversationID, &m.Project, &m.TaskID, &m.Priority, &m.TTLSeconds, &m.ExpiredAt,
			&deliveryID, &deliveryState); err != nil {
			rows.Close()
			return nil, err
		}
		m.DeliveryID = &deliveryID
		m.DeliveryState = &deliveryState
		msgs = append(msgs, m)
		if deliveryState == "queued" {
			queuedIDs = append(queuedIDs, deliveryID)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(queuedIDs) > 0 {
		now := nowMicro()
		if err := s.write(func(tx *sql.Tx) error {
			for _, id := range queuedIDs {
				if _, err := tx.Exec("UPDATE deliveries SET state = 'surfaced', surfaced_at = ? WHERE id = ? AND state = 'queued'", now, id); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return msgs, nil
}

// markRead records read receipts and acknowledges the matching deliveries.
// count is ONLY the newly-inserted receipts (re-marking the same id returns 0).
func (s *Store) markRead(messageIDs []string, agent, project string) (int, error) {
	readAt := nowRFC3339() // message_reads.read_at is RFC3339
	ackAt := nowMicro()    // deliveries timestamps are microsecond
	count := 0
	err := s.write(func(tx *sql.Tx) error {
		for _, id := range messageIDs {
			res, err := tx.Exec(
				"INSERT OR IGNORE INTO message_reads (message_id, agent_name, project, read_at) VALUES (?, ?, ?, ?)",
				id, agent, project, readAt)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			count += int(n)
			if _, err := tx.Exec(
				"UPDATE deliveries SET state = 'acknowledged', acknowledged_at = ? WHERE message_id = ? AND to_agent = ? AND project = ? AND state IN ('queued', 'surfaced')",
				ackAt, id, agent, project); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// --- handlers ---

func handleSendMessage(s *Server, args map[string]any) (toolResult, error) {
	to := strings.ToLower(argString(args, "to"))
	if to == "" {
		return resultError("to is required"), nil
	}
	if argString(args, "content") == "" {
		return resultError("content is required"), nil
	}
	msg, err := s.store.sendMessage(
		resolveProject(args),
		resolveAgent(args),
		to,
		argStringDefault(args, "type", "notification"),
		argString(args, "subject"),
		argString(args, "content"),
		argStringDefault(args, "metadata", "{}"),
		mapPriority(argString(args, "priority")),
		optionalString(argString(args, "reply_to")),
		optionalString(argString(args, "conversation_id")),
	)
	if err != nil {
		return toolResult{}, err
	}
	return resultText(msg)
}

func handleGetInbox(s *Server, args map[string]any) (toolResult, error) {
	agent := resolveAgent(args)
	entries, err := s.store.getInbox(
		resolveProject(args),
		agent,
		argBool(args, "unread_only", true),
		argInt(args, "limit", 10),
	)
	if err != nil {
		return toolResult{}, err
	}

	formatted := formatInboxEntries(entries, argBool(args, "full_content", false))
	return resultText(map[string]any{"agent": agent, "count": len(entries), "messages": formatted})
}

// formatInboxEntries renders inbox messages for the get_inbox wire (matching
// wrai.th's entry keys). Content is truncated to 300 chars unless fullContent.
func formatInboxEntries(msgs []Message, fullContent bool) []map[string]any {
	formatted := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		content := m.Content
		if !fullContent && len(content) > 300 {
			content = content[:300] + "..."
		}
		entry := map[string]any{
			"id": m.ID, "from": m.From, "to": m.To, "type": m.Type,
			"subject": m.Subject, "content": content, "created_at": m.CreatedAt, "priority": m.Priority,
		}
		if m.ReplyTo != nil {
			entry["reply_to"] = *m.ReplyTo
		}
		if m.ConversationID != nil {
			entry["conversation_id"] = *m.ConversationID
		}
		if m.DeliveryID != nil {
			entry["delivery_id"] = *m.DeliveryID
		}
		if m.DeliveryState != nil {
			entry["delivery_state"] = *m.DeliveryState
		}
		formatted[i] = entry
	}
	return formatted
}

func handleMarkRead(s *Server, args map[string]any) (toolResult, error) {
	ids := argStringSlice(args, "message_ids")
	if len(ids) == 0 {
		return resultError("message_ids or conversation_id is required"), nil
	}
	count, err := s.store.markRead(ids, resolveAgent(args), resolveProject(args))
	if err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{"marked_read": count})
}
