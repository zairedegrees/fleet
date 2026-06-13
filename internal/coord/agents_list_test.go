package coord

import (
	"net/http/httptest"
	"testing"

	"github.com/zairedegrees/fleet/internal/relay"
)

func TestListAgentsFiltersAndOrders(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"name": "zeta", "project": "p", "profile_slug": "z"})
	mustCall(t, s, "register_agent", map[string]any{"name": "alpha", "project": "p", "profile_slug": "a"})
	mustCall(t, s, "register_agent", map[string]any{"name": "other", "project": "q"}) // different project
	mustCall(t, s, "deactivate_agent", map[string]any{"name": "zeta", "project": "p"})

	res := mustCall(t, s, "list_agents", map[string]any{"project": "p"})
	var out struct {
		Count  int     `json:"count"`
		Agents []Agent `json:"agents"`
	}
	decodePayload(t, res, &out)

	if len(out.Agents) != 2 {
		t.Fatalf("got %d agents, want 2 (project-scoped)", len(out.Agents))
	}
	if out.Agents[0].Name != "alpha" || out.Agents[1].Name != "zeta" {
		t.Errorf("order = [%s, %s], want [alpha, zeta]", out.Agents[0].Name, out.Agents[1].Name)
	}
	if out.Count != 2 {
		t.Errorf("count = %d, want 2", out.Count)
	}
	if out.Agents[0].ProfileSlug == nil || *out.Agents[0].ProfileSlug != "a" {
		t.Errorf("profile_slug not populated: %v", out.Agents[0].ProfileSlug)
	}
	if out.Agents[1].Status != "inactive" {
		t.Errorf("deactivated agent status = %q, want inactive (still listed)", out.Agents[1].Status)
	}
}

func TestDeactivateReturnsTrueEvenIfNoop(t *testing.T) {
	s := New(newTestStore(t))
	res := mustCall(t, s, "deactivate_agent", map[string]any{"name": "ghost", "project": "p"})
	var out struct {
		Deactivated bool `json:"deactivated"`
	}
	decodePayload(t, res, &out)
	if !out.Deactivated {
		t.Error("deactivate of a non-existent agent should still report deactivated true")
	}
}

// TestListAgentsThroughClient is a Tier-1 check: coord driven through fleet's
// own relay.Client, proving the inner {agents,count} payload decodes into the
// client's Agent struct (name + profile_slug carried).
func TestListAgentsThroughClient(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Handler())
	defer srv.Close()
	c := relay.NewClient(srv.URL + "/mcp")

	if err := c.RegisterAgent("ops", "proj", "monitor", "ops-slug"); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	agents, err := c.ListAgents("proj")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "ops" || agents[0].ProfileSlug != "ops-slug" {
		t.Fatalf("unexpected agents: %+v", agents)
	}
}
