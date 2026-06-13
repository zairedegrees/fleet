package coord

import (
	"database/sql"
	"fmt"

	"github.com/zairedegrees/fleet/internal/coord/normalize"
)

const taskColumns = "id, profile_slug, assigned_to, dispatched_by, title, description, priority, status, result, blocked_reason, project, dispatched_at, accepted_at, started_at, completed_at, parent_task_id, ack_notified_at, ack_escalated_at, board_id, goal_id, archived_at"

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.ProfileSlug, &t.AssignedTo, &t.DispatchedBy, &t.Title, &t.Description,
		&t.Priority, &t.Status, &t.Result, &t.BlockedReason, &t.Project, &t.DispatchedAt,
		&t.AcceptedAt, &t.StartedAt, &t.CompletedAt, &t.ParentTaskID, &t.AckNotifiedAt,
		&t.AckEscalatedAt, &t.BoardID, &t.GoalID, &t.ArchivedAt)
	return t, err
}

// dispatchTask inserts a pending task routed to profileSlug, then fans out an
// inbox notification to every active agent running that profile except the
// dispatcher. Task insert + notifications are one serialized transaction.
func (s *Store) dispatchTask(project, profileSlug, dispatchedBy, title, description, priority string) (*Task, error) {
	if priority == "" {
		priority = "P2"
	}
	now := nowMicro()
	task := &Task{
		ID: newID(), ProfileSlug: profileSlug, DispatchedBy: dispatchedBy, Title: title,
		Description: description, Priority: priority, Status: "pending", Project: project, DispatchedAt: now,
	}

	err := s.write(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			"INSERT INTO tasks (id, profile_slug, dispatched_by, title, description, priority, status, project, dispatched_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			task.ID, task.ProfileSlug, task.DispatchedBy, task.Title, task.Description, task.Priority, task.Status, task.Project, task.DispatchedAt); err != nil {
			return err
		}

		rows, err := tx.Query("SELECT name FROM agents WHERE project = ? AND profile_slug = ? AND status = 'active'", project, profileSlug)
		if err != nil {
			return err
		}
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				return err
			}
			names = append(names, n)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, n := range names {
			if n == dispatchedBy {
				continue // don't notify the dispatcher
			}
			subject := "New task: " + title
			content := fmt.Sprintf("[%s] %s\n\nTask ID: %s\nProfile: %s\nDispatched by: %s", priority, title, task.ID, profileSlug, dispatchedBy)
			if description != "" && len(description) <= 200 {
				content += "\n\n" + description
			}
			if err := insertMessageTx(tx, project, dispatchedBy, n, "task", subject, content, fmt.Sprintf(`{"task_id":"%s"}`, task.ID), "P2"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

// insertMessageTx inserts an inbox message. content/metadata are key-normalized
// like wrai.th's InsertMessage; non-JSON content passes through unchanged.
func insertMessageTx(tx *sql.Tx, project, from, to, msgType, subject, content, metadata, priority string) error {
	_, err := tx.Exec(
		"INSERT INTO messages (id, from_agent, to_agent, reply_to, type, subject, content, metadata, created_at, conversation_id, project, priority, ttl_seconds) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		newID(), from, to, nil, msgType, subject, normalize.JSONKeys(content), normalize.JSONKeys(metadata), nowMicro(), nil, project, priority, 14400)
	return err
}

// listTasks mirrors wrai.th: default limit 50, non-archived, status "active"
// excludes done/cancelled, ordered by P0..P3 then dispatched_at DESC, capped by
// LIMIT. count is taken from the returned page (after the cap) by the handler.
func (s *Store) listTasks(project, status, profileSlug, priority, assignedTo string, limit int) ([]Task, error) {
	if limit <= 0 {
		limit = 50
	}

	query := "SELECT " + taskColumns + " FROM tasks WHERE project = ? AND archived_at IS NULL"
	args := []any{project}

	if status == "active" {
		query += " AND status NOT IN ('done', 'cancelled')"
	} else if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	if profileSlug != "" {
		query += " AND profile_slug = ?"
		args = append(args, profileSlug)
	}
	if priority != "" {
		query += " AND priority = ?"
		args = append(args, priority)
	}
	if assignedTo != "" {
		query += " AND assigned_to = ?"
		args = append(args, assignedTo)
	}

	query += " ORDER BY CASE priority WHEN 'P0' THEN 0 WHEN 'P1' THEN 1 WHEN 'P2' THEN 2 WHEN 'P3' THEN 3 END, dispatched_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.reader().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// --- handlers ---

func handleDispatchTask(s *Server, args map[string]any) (toolResult, error) {
	// The fleet client and the agent skill both send the routing target under the
	// key "profile" (NOT "profile_slug") — reading the wrong key routes every task
	// to slug "" and is the silent-failure trap this is pinned against.
	profile := argString(args, "profile")
	if profile == "" {
		return resultError("profile is required"), nil
	}
	title := argString(args, "title")
	if title == "" {
		return resultError("title is required"), nil
	}
	task, err := s.store.dispatchTask(
		resolveProject(args),
		profile,
		resolveAgent(args),
		title,
		argString(args, "description"),
		argStringDefault(args, "priority", "P2"),
	)
	if err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{"task": task})
}

func handleListTasks(s *Server, args map[string]any) (toolResult, error) {
	tasks, err := s.store.listTasks(
		resolveProject(args),
		argString(args, "status"),
		argString(args, "profile"), // same key as dispatch_task
		argString(args, "priority"),
		argString(args, "assigned_to"),
		argInt(args, "limit", 50),
	)
	if err != nil {
		return toolResult{}, err
	}

	// Truncate to save tokens in the list view; get_task returns the full text.
	// Byte-slice to 200 matches wrai.th exactly.
	for i := range tasks {
		if len(tasks[i].Description) > 200 {
			tasks[i].Description = tasks[i].Description[:200] + "…"
		}
		if tasks[i].Result != nil && len(*tasks[i].Result) > 200 {
			truncated := (*tasks[i].Result)[:200] + "…"
			tasks[i].Result = &truncated
		}
	}

	return resultText(map[string]any{"count": len(tasks), "tasks": tasks})
}
