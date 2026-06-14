package coord

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/zairedegrees/fleet/internal/coord/normalize"
)

const memoryColumns = "id, key, value, tags, scope, project, agent_name, confidence, version, supersedes, conflict_with, created_at, updated_at, archived_at, archived_by, layer"

func scanMemory(row interface{ Scan(...any) error }) (Memory, error) {
	var m Memory
	err := row.Scan(&m.ID, &m.Key, &m.Value, &m.Tags, &m.Scope, &m.Project, &m.AgentName,
		&m.Confidence, &m.Version, &m.Supersedes, &m.ConflictWith, &m.CreatedAt, &m.UpdatedAt,
		&m.ArchivedAt, &m.ArchivedBy, &m.Layer)
	return m, err
}

func tagsToJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// findActiveMemoryTx returns the highest-version non-archived memory at the
// given scope+key (scope-appropriate filtering), or nil.
func findActiveMemoryTx(tx *sql.Tx, project, scope, agentName, key string) (*Memory, error) {
	var query string
	var args []any
	switch scope {
	case "agent":
		query = "SELECT " + memoryColumns + " FROM memories WHERE key = ? AND scope = 'agent' AND project = ? AND agent_name = ? AND archived_at IS NULL ORDER BY version DESC LIMIT 1"
		args = []any{key, project, agentName}
	case "global":
		query = "SELECT " + memoryColumns + " FROM memories WHERE key = ? AND scope = 'global' AND archived_at IS NULL ORDER BY version DESC LIMIT 1"
		args = []any{key}
	case "project":
		query = "SELECT " + memoryColumns + " FROM memories WHERE key = ? AND scope = 'project' AND project = ? AND archived_at IS NULL ORDER BY version DESC LIMIT 1"
		args = []any{key, project}
	default:
		return nil, fmt.Errorf("invalid scope: %s", scope)
	}
	m, err := scanMemory(tx.QueryRow(query, args...))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// setMemory writes a memory with versioning. Same value → touch only. Different
// value with upsert → archive the old version, insert v+1 (supersedes). Different
// value without upsert → insert v+1 flagged conflict_with the old version. The
// value is key-normalized (no-op for opaque/non-JSON content like vault docs).
func (s *Store) setMemory(project, agent, key, value, tagsJSON, scope, confidence, layer string, upsert bool) (*Memory, error) {
	value = normalize.JSONKeys(value)
	now := nowMicro()
	if confidence == "" {
		confidence = "stated"
	}
	if tagsJSON == "" {
		tagsJSON = "[]"
	}
	if layer == "" {
		layer = "behavior"
	}

	var result *Memory
	err := s.write(func(tx *sql.Tx) error {
		existing, err := findActiveMemoryTx(tx, project, scope, agent, key)
		if err != nil {
			return err
		}
		id := newID()

		if existing != nil {
			if existing.Value == value {
				if _, err := tx.Exec("UPDATE memories SET updated_at = ?, tags = ?, confidence = ? WHERE id = ?", now, tagsJSON, confidence, existing.ID); err != nil {
					return err
				}
				existing.UpdatedAt = now
				existing.Tags = tagsJSON
				existing.Confidence = confidence
				result = existing
				return nil
			}

			m := &Memory{
				ID: id, Key: key, Value: value, Tags: tagsJSON, Scope: scope, Project: project,
				AgentName: agent, Confidence: confidence, Version: existing.Version + 1,
				Supersedes: &existing.ID, CreatedAt: now, UpdatedAt: now, Layer: layer,
			}
			if upsert {
				if _, err := tx.Exec("UPDATE memories SET archived_at = ?, archived_by = ? WHERE id = ?", now, "upsert", existing.ID); err != nil {
					return err
				}
				if _, err := tx.Exec(
					"INSERT INTO memories (id, key, value, tags, scope, project, agent_name, confidence, version, supersedes, created_at, updated_at, layer) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
					m.ID, m.Key, m.Value, m.Tags, m.Scope, m.Project, m.AgentName, m.Confidence, m.Version, m.Supersedes, m.CreatedAt, m.UpdatedAt, m.Layer); err != nil {
					return err
				}
				result = m
				return nil
			}

			m.ConflictWith = &existing.ID
			if _, err := tx.Exec(
				"INSERT INTO memories (id, key, value, tags, scope, project, agent_name, confidence, version, supersedes, conflict_with, created_at, updated_at, layer) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				m.ID, m.Key, m.Value, m.Tags, m.Scope, m.Project, m.AgentName, m.Confidence, m.Version, m.Supersedes, m.ConflictWith, m.CreatedAt, m.UpdatedAt, m.Layer); err != nil {
				return err
			}
			result = m
			return nil
		}

		m := &Memory{
			ID: id, Key: key, Value: value, Tags: tagsJSON, Scope: scope, Project: project,
			AgentName: agent, Confidence: confidence, Version: 1, CreatedAt: now, UpdatedAt: now, Layer: layer,
		}
		if _, err := tx.Exec(
			"INSERT INTO memories (id, key, value, tags, scope, project, agent_name, confidence, version, created_at, updated_at, layer) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			m.ID, m.Key, m.Value, m.Tags, m.Scope, m.Project, m.AgentName, m.Confidence, m.Version, m.CreatedAt, m.UpdatedAt, m.Layer); err != nil {
			return err
		}
		result = m
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// getMemory retrieves a key, cascading agent → project → global when no scope is
// given (first non-empty scope wins); an explicit scope searches only that scope.
func (s *Store) getMemory(project, agent, key, scope string) ([]Memory, error) {
	if scope != "" {
		return s.getMemoryAtScope(project, agent, key, scope)
	}
	for _, sc := range []string{"agent", "project", "global"} {
		res, err := s.getMemoryAtScope(project, agent, key, sc)
		if err != nil {
			return nil, err
		}
		if len(res) > 0 {
			return res, nil
		}
	}
	return []Memory{}, nil
}

func (s *Store) getMemoryAtScope(project, agent, key, scope string) ([]Memory, error) {
	var query string
	var args []any
	switch scope {
	case "agent":
		query = "SELECT " + memoryColumns + " FROM memories WHERE key = ? AND scope = 'agent' AND project = ? AND agent_name = ? AND archived_at IS NULL ORDER BY version DESC"
		args = []any{key, project, agent}
	case "project":
		query = "SELECT " + memoryColumns + " FROM memories WHERE key = ? AND scope = 'project' AND project = ? AND archived_at IS NULL ORDER BY version DESC"
		args = []any{key, project}
	case "global":
		query = "SELECT " + memoryColumns + " FROM memories WHERE key = ? AND scope = 'global' AND archived_at IS NULL ORDER BY version DESC"
		args = []any{key}
	default:
		return nil, fmt.Errorf("invalid scope: %s", scope)
	}

	rows, err := s.reader().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mems := []Memory{}
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		mems = append(mems, m)
	}
	return mems, rows.Err()
}

// --- handlers ---

func handleSetMemory(s *Server, args map[string]any) (toolResult, error) {
	key := argString(args, "key")
	if key == "" {
		return resultError("key is required"), nil
	}
	value := argString(args, "value")
	if value == "" {
		return resultError("value is required"), nil
	}
	mem, err := s.store.setMemory(
		resolveProject(args),
		resolveAgent(args),
		key,
		value,
		tagsToJSON(argStringSlice(args, "tags")),
		argStringDefault(args, "scope", "project"),
		argStringDefault(args, "confidence", "stated"),
		argStringDefault(args, "layer", "behavior"),
		argBool(args, "upsert", true),
	)
	if err != nil {
		return toolResult{}, err
	}
	result := map[string]any{"memory": mem}
	if mem.ConflictWith != nil {
		result["conflict"] = true
		result["message"] = fmt.Sprintf("Conflict detected: key '%s' already exists with a different value. Both versions preserved. Use resolve_conflict to pick the truth.", key)
	}
	return resultText(result)
}

func handleGetMemory(s *Server, args map[string]any) (toolResult, error) {
	key := argString(args, "key")
	if key == "" {
		return resultError("key is required"), nil
	}
	mems, err := s.store.getMemory(resolveProject(args), resolveAgent(args), key, argString(args, "scope"))
	if err != nil {
		return toolResult{}, err
	}
	result := map[string]any{"key": key, "count": len(mems), "memories": mems}
	if len(mems) > 1 {
		result["conflict"] = true
		result["message"] = "Multiple values exist for this key. Use resolve_conflict to pick the truth."
	}
	return resultText(result)
}
