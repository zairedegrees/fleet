package coord

import (
	"database/sql"
	"strings"
)

const agentColumns = "id, name, role, description, registered_at, last_seen, project, reports_to, profile_slug, status, deactivated_at, is_executive, session_id, interest_tags, max_context_bytes"

func scanAgent(row interface{ Scan(...any) error }) (Agent, error) {
	var a Agent
	err := row.Scan(&a.ID, &a.Name, &a.Role, &a.Description, &a.RegisteredAt, &a.LastSeen,
		&a.Project, &a.ReportsTo, &a.ProfileSlug, &a.Status, &a.DeactivatedAt, &a.IsExecutive,
		&a.SessionID, &a.InterestTags, &a.MaxContextBytes)
	return a, err
}

// registerOptions carries presence flags for identity fields. On a respawn, a
// field that was NOT provided is preserved from the existing row instead of
// being clobbered to NULL/false — the fix for the profile_slug-goes-NULL bug.
type registerOptions struct {
	reportsToSet   bool
	profileSlugSet bool
	isExecutiveSet bool
	sessionIDSet   bool
}

// registerAgent inserts a new agent or, on respawn, updates it while preserving
// omitted identity fields. The whole read-then-write runs in one serialized
// transaction so a concurrent re-register can't see a stale row.
func (s *Store) registerAgent(project, name, role, description string, reportsTo, profileSlug *string, isExecutive bool, sessionID *string, interestTags string, maxContextBytes int, opts registerOptions) (*Agent, bool, error) {
	now := nowRFC3339()
	if interestTags == "" {
		interestTags = "[]"
	}
	if maxContextBytes <= 0 {
		maxContextBytes = 16384
	}

	var result *Agent
	var isRespawn bool
	err := s.write(func(tx *sql.Tx) error {
		ensureProjectTx(tx, project)

		existing, err := scanAgent(tx.QueryRow("SELECT "+agentColumns+" FROM agents WHERE name = ? AND project = ?", name, project))
		if err == sql.ErrNoRows {
			a := &Agent{
				ID: newID(), Name: name, Role: role, Description: description,
				RegisteredAt: now, LastSeen: now, Project: project,
				ReportsTo: reportsTo, ProfileSlug: profileSlug, Status: "active",
				IsExecutive: isExecutive, SessionID: sessionID,
				InterestTags: interestTags, MaxContextBytes: maxContextBytes,
			}
			if _, err := tx.Exec("INSERT INTO agents ("+agentColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				a.ID, a.Name, a.Role, a.Description, a.RegisteredAt, a.LastSeen, a.Project,
				a.ReportsTo, a.ProfileSlug, a.Status, a.DeactivatedAt, a.IsExecutive,
				a.SessionID, a.InterestTags, a.MaxContextBytes); err != nil {
				return err
			}
			result = a
			return applyLeadershipTx(tx, project, name, a.IsExecutive)
		}
		if err != nil {
			return err
		}

		// Respawn: preserve identity fields not provided on this call.
		if !opts.reportsToSet {
			reportsTo = existing.ReportsTo
		}
		if !opts.profileSlugSet {
			profileSlug = existing.ProfileSlug
		}
		if !opts.isExecutiveSet {
			isExecutive = existing.IsExecutive
		}
		if !opts.sessionIDSet {
			sessionID = existing.SessionID
		}

		if _, err := tx.Exec(
			"UPDATE agents SET role = ?, description = ?, last_seen = ?, reports_to = ?, profile_slug = ?, is_executive = ?, session_id = ?, interest_tags = ?, max_context_bytes = ?, status = 'active', deactivated_at = NULL WHERE name = ? AND project = ?",
			role, description, now, reportsTo, profileSlug, isExecutive, sessionID, interestTags, maxContextBytes, name, project); err != nil {
			return err
		}

		existing.Role = role
		existing.Description = description
		existing.LastSeen = now
		existing.ReportsTo = reportsTo
		existing.ProfileSlug = profileSlug
		existing.IsExecutive = isExecutive
		existing.SessionID = sessionID
		existing.InterestTags = interestTags
		existing.MaxContextBytes = maxContextBytes
		existing.Status = "active"
		existing.DeactivatedAt = nil
		result = &existing
		isRespawn = true
		return applyLeadershipTx(tx, project, name, existing.IsExecutive)
	})
	if err != nil {
		return nil, false, err
	}
	return result, isRespawn, nil
}

func (s *Store) listAgents(project string) ([]Agent, error) {
	rows, err := s.reader().Query(
		"SELECT "+agentColumns+" FROM agents WHERE project = ? AND status IN ('active', 'sleeping', 'inactive') ORDER BY name",
		project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := []Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *Store) deactivateAgent(project, name string) error {
	now := nowRFC3339()
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"UPDATE agents SET status = 'inactive', deactivated_at = ? WHERE name = ? AND project = ? AND status IN ('active', 'sleeping')",
			now, name, project)
		return err
	})
}

func (s *Store) getAgent(project, name string) (*Agent, error) {
	a, err := scanAgent(s.reader().QueryRow("SELECT "+agentColumns+" FROM agents WHERE name = ? AND project = ?", name, project))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ensureProjectTx creates the project row on first use (matching wrai.th's
// auto-create); the routing tables don't FK to it, but keeping the row makes
// later project-scoped reads consistent.
func ensureProjectTx(tx *sql.Tx, project string) {
	_, _ = tx.Exec("INSERT OR IGNORE INTO projects (name, created_at) VALUES (?, ?)", project, nowRFC3339())
}

// applyLeadershipTx reproduces the is_executive side effect: a leadership team
// (type admin) is auto-created per project and the agent joins it as admin. A
// respawn that omits is_executive still re-drives this from the merged value.
func applyLeadershipTx(tx *sql.Tx, project, agentName string, isExecutive bool) error {
	if !isExecutive {
		return nil
	}
	var teamID string
	err := tx.QueryRow("SELECT id FROM teams WHERE project = ? AND slug = 'leadership'", project).Scan(&teamID)
	if err == sql.ErrNoRows {
		teamID = newID()
		if _, err := tx.Exec(
			"INSERT INTO teams (id, name, slug, org_id, project, description, type, parent_team_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			teamID, "Leadership", "leadership", nil, project, "Auto-created admin team for executive agents", "admin", nil, nowRFC3339()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// OR REPLACE matches wrai.th's AddTeamMember: a respawn re-drive refreshes the
	// membership row (role/joined_at) rather than leaving a stale one.
	_, err = tx.Exec(
		"INSERT OR REPLACE INTO team_members (team_id, agent_name, project, role, joined_at) VALUES (?, ?, ?, ?, ?)",
		teamID, strings.ToLower(agentName), project, "admin", nowRFC3339())
	return err
}

// --- handlers ---

func handleRegisterAgent(s *Server, args map[string]any) (toolResult, error) {
	name := strings.ToLower(argString(args, "name"))
	if name == "" {
		return resultError("name is required"), nil
	}
	project := resolveProject(args)
	role := argString(args, "role")
	description := argString(args, "description")
	reportsTo := optionalStringLower(argString(args, "reports_to"))
	profileSlug := optionalString(argString(args, "profile_slug"))
	isExecutive := argBool(args, "is_executive", false)
	sessionID := optionalString(argString(args, "session_id"))
	interestTags := argStringDefault(args, "interest_tags", "[]")
	maxContextBytes := argInt(args, "max_context_bytes", 16384)

	opts := registerOptions{
		reportsToSet:   argPresent(args, "reports_to"),
		profileSlugSet: argPresent(args, "profile_slug"),
		isExecutiveSet: argPresent(args, "is_executive"),
		sessionIDSet:   argPresent(args, "session_id"),
	}

	agent, isRespawn, err := s.store.registerAgent(project, name, role, description, reportsTo, profileSlug, isExecutive, sessionID, interestTags, maxContextBytes, opts)
	if err != nil {
		return toolResult{}, err
	}
	resp := map[string]any{
		"agent":           agent,
		"session_context": map[string]any{"is_respawn": isRespawn},
	}
	// Executives are auto-added to the leadership admin team; surface the same
	// two response keys wrai.th does so a broadcast-capable agent knows.
	if agent.IsExecutive {
		resp["auto_admin_team"] = "leadership"
		resp["hint"] = "You were auto-added to the 'leadership' admin team (broadcast enabled). Use send_message(to='*') to broadcast."
	}
	return resultText(resp)
}

func handleListAgents(s *Server, args map[string]any) (toolResult, error) {
	agents, err := s.store.listAgents(resolveProject(args))
	if err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{"count": len(agents), "agents": agents})
}

func handleDeactivateAgent(s *Server, args map[string]any) (toolResult, error) {
	name := strings.ToLower(argString(args, "name"))
	if name == "" {
		return resultError("name is required"), nil
	}
	if err := s.store.deactivateAgent(resolveProject(args), name); err != nil {
		return toolResult{}, err
	}
	return resultText(map[string]any{"deactivated": true, "agent": name})
}
