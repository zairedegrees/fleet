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
// It returns the task and the list of agent names that were notified.
func (s *Store) dispatchTask(project, profileSlug, dispatchedBy, title, description, priority, goalID string) (*Task, []string, error) {
	if priority == "" {
		priority = "P2"
	}
	now := nowMicro()
	task := &Task{
		ID: newID(), ProfileSlug: profileSlug, DispatchedBy: dispatchedBy, Title: title,
		Description: description, Priority: priority, Status: "pending", Project: project, DispatchedAt: now,
		GoalID: optionalString(goalID),
	}

	var notified []string
	err := s.write(func(tx *sql.Tx) error {
		if goalID != "" {
			var one int
			err := tx.QueryRow("SELECT 1 FROM goals WHERE id = ? AND project = ?", goalID, project).Scan(&one)
			if err == sql.ErrNoRows {
				return fmt.Errorf("unknown goal_id %q — create_goal first", goalID)
			}
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(
			"INSERT INTO tasks (id, profile_slug, dispatched_by, title, description, priority, status, project, dispatched_at, goal_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			task.ID, task.ProfileSlug, task.DispatchedBy, task.Title, task.Description, task.Priority, task.Status, task.Project, task.DispatchedAt, task.GoalID); err != nil {
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
			notified = append(notified, n)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return task, notified, nil
}

// insertMessageTx inserts an inbox message and queues its delivery so it shows
// up in the recipient's delivery-based inbox. content/metadata are key-normalized
// like wrai.th's InsertMessage; non-JSON content passes through unchanged.
func insertMessageTx(tx *sql.Tx, project, from, to, msgType, subject, content, metadata, priority string) error {
	msgID := newID()
	if _, err := tx.Exec(
		"INSERT INTO messages (id, from_agent, to_agent, reply_to, type, subject, content, metadata, created_at, conversation_id, project, priority, ttl_seconds) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		msgID, from, to, nil, msgType, subject, normalize.JSONKeys(content), normalize.JSONKeys(metadata), nowMicro(), nil, project, priority, 14400); err != nil {
		return err
	}
	return createDeliveryTx(tx, msgID, project, []string{to})
}

// listTasks mirrors wrai.th: default limit 50, non-archived, status "active"
// excludes done/cancelled, ordered by P0..P3 then dispatched_at DESC, capped by
// LIMIT. count is taken from the returned page (after the cap) by the handler.
func (s *Store) listTasks(project, status, profileSlug, priority, assignedTo, goalID string, limit int) ([]Task, error) {
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
	if goalID != "" {
		query += " AND goal_id = ?"
		args = append(args, goalID)
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

// validTransitions is the task state machine. An agent move must be in the
// allowed set for the current status; the special agent name "user" bypasses
// validation (admin force-move).
var validTransitions = map[string][]string{
	"pending":     {"accepted", "in-progress", "done", "cancelled"},
	"accepted":    {"in-progress", "done", "cancelled"},
	"in-progress": {"done", "blocked", "cancelled"},
	"blocked":     {"in-progress", "done", "cancelled"},
	"done":        {"cancelled"},
	"cancelled":   {},
}

// normalizePtr key-normalizes a task result string (no-op on nil / non-JSON).
func normalizePtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := normalize.JSONKeys(*s)
	return &v
}

// transitionTask moves a task to newStatus inside one serialized transaction,
// validating the move (unless agentName is "user") and applying the per-status
// timestamp/field changes that wrai.th does.
func (s *Store) transitionTask(taskID, agentName, project, newStatus string, result, blockedReason *string) (*Task, error) {
	now := nowMicro()
	var out *Task
	err := s.write(func(tx *sql.Tx) error {
		task, err := scanTask(tx.QueryRow("SELECT "+taskColumns+" FROM tasks WHERE id = ? AND project = ?", taskID, project))
		if err == sql.ErrNoRows {
			return fmt.Errorf("task not found: %s", taskID)
		}
		if err != nil {
			return err
		}

		if agentName != "user" {
			valid := false
			for _, st := range validTransitions[task.Status] {
				if st == newStatus {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("invalid transition: %s → %s", task.Status, newStatus)
			}
		}

		task.Status = newStatus
		switch newStatus {
		case "accepted":
			task.AssignedTo = &agentName
			task.AcceptedAt = &now
			_, err = tx.Exec("UPDATE tasks SET status = ?, assigned_to = ?, accepted_at = ? WHERE id = ? AND project = ?",
				newStatus, agentName, now, taskID, project)
		case "in-progress":
			task.AssignedTo = &agentName
			task.StartedAt = &now
			_, err = tx.Exec("UPDATE tasks SET status = ?, assigned_to = ?, started_at = ? WHERE id = ? AND project = ?",
				newStatus, agentName, now, taskID, project)
		case "done":
			result = normalizePtr(result)
			task.Result = result
			task.CompletedAt = &now
			_, err = tx.Exec("UPDATE tasks SET status = ?, result = ?, completed_at = ? WHERE id = ? AND project = ?",
				newStatus, result, now, taskID, project)
		case "blocked":
			task.BlockedReason = blockedReason
			_, err = tx.Exec("UPDATE tasks SET status = ?, blocked_reason = ? WHERE id = ? AND project = ?",
				newStatus, blockedReason, taskID, project)
		case "cancelled":
			task.BlockedReason = blockedReason
			task.CompletedAt = &now
			_, err = tx.Exec("UPDATE tasks SET status = ?, blocked_reason = ?, completed_at = ? WHERE id = ? AND project = ?",
				newStatus, blockedReason, now, taskID, project)
		case "pending":
			task.AssignedTo, task.AcceptedAt, task.StartedAt = nil, nil, nil
			task.CompletedAt, task.Result, task.BlockedReason = nil, nil, nil
			_, err = tx.Exec("UPDATE tasks SET status = ?, assigned_to = NULL, accepted_at = NULL, started_at = NULL, completed_at = NULL, result = NULL, blocked_reason = NULL WHERE id = ? AND project = ?",
				newStatus, taskID, project)
		}
		if err != nil {
			return err
		}
		out = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) getTask(taskID, project string) (*Task, error) {
	t, err := scanTask(s.reader().QueryRow("SELECT "+taskColumns+" FROM tasks WHERE id = ? AND project = ?", taskID, project))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// resolveTaskID accepts a full UUID or a unique prefix (agents often paste a
// short id). An ambiguous prefix is an error — never guess at which task a
// lifecycle move targets — matching wrai.th's refuse-on-collision behavior.
func (s *Store) resolveTaskID(taskID, project string) (string, error) {
	if len(taskID) >= 36 {
		return taskID, nil
	}
	rows, err := s.reader().Query("SELECT id FROM tasks WHERE project = ? AND id LIKE ?", project, taskID+"%")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("task not found: %s", taskID)
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("ambiguous task ID prefix %q (%d matches)", taskID, len(ids))
	}
}

// --- handlers ---

// resolveTaskArg pulls and resolves task_id; on failure it returns the error
// result to send and ok=false.
func resolveTaskArg(s *Server, args map[string]any) (id, project string, errRes toolResult, ok bool) {
	project = resolveProject(args)
	taskID := argString(args, "task_id")
	if taskID == "" {
		return "", project, resultError("task_id is required"), false
	}
	resolved, err := s.store.resolveTaskID(taskID, project)
	if err != nil {
		return "", project, resultError(err.Error()), false
	}
	return resolved, project, toolResult{}, true
}

func handleClaimTask(s *Server, args map[string]any) (toolResult, error) {
	id, project, errRes, ok := resolveTaskArg(s, args)
	if !ok {
		return errRes, nil
	}
	task, err := s.store.transitionTask(id, resolveAgent(args), project, "accepted", nil, nil)
	if err != nil {
		return resultError(err.Error()), nil
	}
	return resultText(task)
}

func handleStartTask(s *Server, args map[string]any) (toolResult, error) {
	id, project, errRes, ok := resolveTaskArg(s, args)
	if !ok {
		return errRes, nil
	}
	task, err := s.store.transitionTask(id, resolveAgent(args), project, "in-progress", nil, nil)
	if err != nil {
		return resultError(err.Error()), nil
	}
	return resultText(task)
}

func handleCompleteTask(s *Server, args map[string]any) (toolResult, error) {
	id, project, errRes, ok := resolveTaskArg(s, args)
	if !ok {
		return errRes, nil
	}
	task, err := s.store.transitionTask(id, resolveAgent(args), project, "done", optionalString(argString(args, "result")), nil)
	if err != nil {
		return resultError(err.Error()), nil
	}
	return resultText(task)
}

func handleBlockTask(s *Server, args map[string]any) (toolResult, error) {
	id, project, errRes, ok := resolveTaskArg(s, args)
	if !ok {
		return errRes, nil
	}
	task, err := s.store.transitionTask(id, resolveAgent(args), project, "blocked", nil, optionalString(argString(args, "reason")))
	if err != nil {
		return resultError(err.Error()), nil
	}
	return resultText(task)
}

func handleGetTask(s *Server, args map[string]any) (toolResult, error) {
	id, project, errRes, ok := resolveTaskArg(s, args)
	if !ok {
		return errRes, nil
	}
	task, err := s.store.getTask(id, project)
	if err != nil {
		return toolResult{}, err
	}
	if task == nil {
		return resultError("task not found: " + id), nil
	}
	return resultText(task)
}

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
	project := resolveProject(args)
	task, notified, err := s.store.dispatchTask(
		project,
		profile,
		resolveAgent(args),
		title,
		argString(args, "description"),
		argStringDefault(args, "priority", "P2"),
		argString(args, "goal_id"),
	)
	if err != nil {
		return toolResult{}, err
	}
	for _, n := range notified {
		s.emitDispatched(project, n)
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
		argString(args, "goal_id"),
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
