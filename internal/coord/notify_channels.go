package coord

import (
	"database/sql"
	"strings"
)

// registerNotifyChannel records a wake target for an agent (the reserved
// agent_notify_channels table). Re-registering the same triple is a no-op.
func (s *Store) registerNotifyChannel(project, agent, target string) error {
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"INSERT OR IGNORE INTO agent_notify_channels (agent_name, project, target) VALUES (?, ?, ?)",
			agent, project, target)
		return err
	})
}

// notifyChannelTarget returns the agent's tmux wake target, if registered.
func (s *Store) notifyChannelTarget(project, agent string) (string, bool, error) {
	var target string
	err := s.reader().QueryRow(
		"SELECT target FROM agent_notify_channels WHERE agent_name = ? AND project = ? AND target LIKE 'tmux:%' LIMIT 1",
		agent, project).Scan(&target)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return target, true, nil
}

// NotifyChannelTarget exposes the tmux wake target for the waker (coordmgr).
func (s *Server) NotifyChannelTarget(project, agent string) (string, bool, error) {
	return s.store.notifyChannelTarget(project, agent)
}

// agentsWithPendingTasks lists recipients of unread task-type messages that have
// a registered tmux notify channel — the sweep's wake candidates. "Unread" means
// the delivery is still 'queued' or 'surfaced': this mirrors getInbox's own
// definition of pending, so an agent that polled its inbox (surfacing the task)
// but never mark_read'd it is still a wake candidate. Only 'acknowledged'
// deliveries (set by markRead) are treated as done and excluded.
func (s *Store) agentsWithPendingTasks() ([]WakeRequest, error) {
	rows, err := s.reader().Query(`
		SELECT DISTINCT d.project, d.to_agent
		FROM deliveries d
		JOIN messages m ON d.message_id = m.id
		JOIN agent_notify_channels c ON c.agent_name = d.to_agent AND c.project = d.project
		WHERE d.state IN ('queued', 'surfaced') AND m.type = 'task' AND m.expired_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WakeRequest{}
	for rows.Next() {
		var w WakeRequest
		if err := rows.Scan(&w.Project, &w.Agent); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// AgentsWithPendingTasks exposes the sweep candidates to the waker (coordmgr).
func (s *Server) AgentsWithPendingTasks() ([]WakeRequest, error) {
	return s.store.agentsWithPendingTasks()
}

// RegisterNotifyChannelForTest seeds a wake channel. Exported for cross-package
// tests (coordmgr); production registration goes through the MCP tool.
func (s *Server) RegisterNotifyChannelForTest(project, agent, target string) error {
	return s.store.registerNotifyChannel(project, agent, target)
}

func handleRegisterNotifyChannel(s *Server, args map[string]any) (toolResult, error) {
	// Lowercase the name to match register_agent / deactivate_agent, so the
	// waker's lookup (keyed on the agent's stored, lowercased name) hits.
	agent := strings.ToLower(argString(args, "name"))
	if agent == "" {
		return resultError("name is required"), nil
	}
	target := argString(args, "target")
	if target == "" {
		return resultError("target is required"), nil
	}
	// Only tmux targets are surfaced by notifyChannelTarget; reject anything
	// else at write time so a typo'd scheme fails loudly instead of registering
	// a silent black hole the waker can never read back.
	if !strings.HasPrefix(target, "tmux:") {
		return resultError("target must start with 'tmux:'"), nil
	}
	if err := s.store.registerNotifyChannel(resolveProject(args), agent, target); err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{"ok": true})
}
