package coord

import "testing"

// TestReRegisterPreservesIdentityOnOmit is the register-preserve gate: a bare
// respawn that omits the identity fields must NOT clobber them to NULL/false.
// This is the regression guard the Tier-1 client replay is blind to (the client
// always sends every field), so it must stay a hard test.
func TestReRegisterPreservesIdentityOnOmit(t *testing.T) {
	s := New(newTestStore(t))

	mustCall(t, s, "register_agent", map[string]any{
		"name": "ops", "project": "p", "role": "monitor",
		"profile_slug": "ops-slug", "reports_to": "cto",
		"is_executive": true, "session_id": "sess-1",
	})

	// Bare respawn: only name/project/role, all identity fields omitted.
	mustCall(t, s, "register_agent", map[string]any{
		"name": "ops", "project": "p", "role": "monitor-v2",
	})

	a, err := s.store.getAgent("p", "ops")
	if err != nil || a == nil {
		t.Fatalf("getAgent: %v / %v", a, err)
	}
	if a.ProfileSlug == nil || *a.ProfileSlug != "ops-slug" {
		t.Errorf("profile_slug not preserved: %v", a.ProfileSlug)
	}
	if a.ReportsTo == nil || *a.ReportsTo != "cto" {
		t.Errorf("reports_to not preserved: %v", a.ReportsTo)
	}
	if !a.IsExecutive {
		t.Error("is_executive not preserved")
	}
	if a.SessionID == nil || *a.SessionID != "sess-1" {
		t.Errorf("session_id not preserved: %v", a.SessionID)
	}
	// Always-update fields and forced status.
	if a.Role != "monitor-v2" {
		t.Errorf("role = %q, want monitor-v2", a.Role)
	}
	if a.Status != "active" {
		t.Errorf("status = %q, want active", a.Status)
	}
}

func TestExplicitFieldsStillUpdate(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{
		"name": "ops", "project": "p",
		"profile_slug": "old", "reports_to": "boss1", "is_executive": true, "session_id": "s1",
	})
	// Every identity field explicitly changed — including is_executive true->false,
	// which must be honored (an explicit false is not "omitted").
	mustCall(t, s, "register_agent", map[string]any{
		"name": "ops", "project": "p",
		"profile_slug": "new", "reports_to": "boss2", "is_executive": false, "session_id": "s2",
	})

	a, _ := s.store.getAgent("p", "ops")
	if a.ProfileSlug == nil || *a.ProfileSlug != "new" {
		t.Errorf("profile_slug not updated: %v", a.ProfileSlug)
	}
	if a.ReportsTo == nil || *a.ReportsTo != "boss2" {
		t.Errorf("reports_to not updated: %v", a.ReportsTo)
	}
	if a.IsExecutive {
		t.Error("explicit is_executive=false was not honored (preserved as true)")
	}
	if a.SessionID == nil || *a.SessionID != "s2" {
		t.Errorf("session_id not updated: %v", a.SessionID)
	}
}

func TestLeadershipMembershipLowercasesName(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"name": "Boss", "project": "p", "is_executive": true})
	if !isLeadershipMember(t, s, "p", "boss") {
		t.Error("executive 'Boss' not added to leadership team under lowercased 'boss'")
	}
}

func TestExecutiveCreatesLeadershipTeamAndSurvivesRespawn(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"name": "boss", "project": "p", "is_executive": true})
	if !isLeadershipMember(t, s, "p", "boss") {
		t.Fatal("executive not added to leadership team on register")
	}

	// Respawn omitting is_executive: it is preserved (true) and the side effect
	// re-drives idempotently — membership must survive.
	mustCall(t, s, "register_agent", map[string]any{"name": "boss", "project": "p", "role": "updated"})
	a, _ := s.store.getAgent("p", "boss")
	if !a.IsExecutive {
		t.Error("is_executive not preserved on respawn")
	}
	if !isLeadershipMember(t, s, "p", "boss") {
		t.Error("leadership membership lost after respawn")
	}
}

func isLeadershipMember(t *testing.T, s *Server, project, name string) bool {
	t.Helper()
	var c int
	err := s.store.reader().QueryRow(
		`SELECT COUNT(*) FROM team_members tm JOIN teams t ON t.id = tm.team_id
		 WHERE t.project = ? AND t.slug = 'leadership' AND tm.agent_name = ?`,
		project, name).Scan(&c)
	if err != nil {
		t.Fatalf("query leadership membership: %v", err)
	}
	return c > 0
}
