package coord

import "database/sql"

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

func handleRegisterNotifyChannel(s *Server, args map[string]any) (toolResult, error) {
	agent := argString(args, "name")
	if agent == "" {
		return resultError("name is required"), nil
	}
	target := argString(args, "target")
	if target == "" {
		return resultError("target is required"), nil
	}
	if err := s.store.registerNotifyChannel(resolveProject(args), agent, target); err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{"ok": true})
}
