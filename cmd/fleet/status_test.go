package main

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/relay"
)

// mergeAgents unions the tmux view with the relay view: a session registered on
// the relay carries the relay status + task count, a session unknown to the
// relay is "unregistered", and a relay agent without a session is a ghost.
func TestMergeAgentsUnionsTmuxAndRelay(t *testing.T) {
	sessions := []string{"fleet-proj-dev", "fleet-proj-ops"}
	relayAgents := []relay.Agent{
		{Name: "dev", Status: "active"},
		{Name: "ghost", Status: "idle"},
	}
	tasks := map[string]int{"dev": 2, "ghost": 0}

	agents := mergeAgents("proj", sessions, relayAgents, tasks, true)
	if len(agents) != 3 {
		t.Fatalf("expected 3 agents (2 sessions + 1 ghost), got %d: %+v", len(agents), agents)
	}

	dev := agents[0]
	if dev.Agent != "dev" || dev.Session != "fleet-proj-dev" || !dev.HasSession {
		t.Errorf("dev should keep its session, got %+v", dev)
	}
	if dev.RelayState != "active" || dev.Tasks != 2 {
		t.Errorf("dev should carry relay state + task count, got %+v", dev)
	}

	ops := agents[1]
	if ops.RelayState != "unregistered" {
		t.Errorf("ops has no relay entry, expected 'unregistered', got %q", ops.RelayState)
	}
	if ops.Tasks != -1 {
		t.Errorf("unregistered agent has no task count, expected -1, got %d", ops.Tasks)
	}

	ghost := agents[2]
	if ghost.Agent != "ghost" || ghost.HasSession {
		t.Errorf("ghost is relay-only, expected HasSession=false, got %+v", ghost)
	}
	if ghost.RelayState != "idle" || ghost.Tasks != 0 {
		t.Errorf("ghost should carry relay state + task count, got %+v", ghost)
	}
}

// When the relay is down we must not invent agent state: every session is
// listed with no relay info at all (empty RelayState, unknown tasks).
func TestMergeAgentsRelayDown(t *testing.T) {
	agents := mergeAgents("proj", []string{"fleet-proj-dev"}, nil, nil, false)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].RelayState != "" {
		t.Errorf("relay down: state must be empty, not %q", agents[0].RelayState)
	}
	if agents[0].Tasks != -1 {
		t.Errorf("relay down: tasks must be unknown (-1), got %d", agents[0].Tasks)
	}
}

// A registered agent whose task count could not be fetched shows an honest
// unknown, never a fake 0.
func TestMergeAgentsMissingTaskCountIsUnknown(t *testing.T) {
	agents := mergeAgents("proj", []string{"fleet-proj-dev"},
		[]relay.Agent{{Name: "dev", Status: "active"}}, map[string]int{}, true)
	if agents[0].Tasks != -1 {
		t.Errorf("missing task count must stay -1, got %d", agents[0].Tasks)
	}
}

func TestRenderStatusFullView(t *testing.T) {
	projects := []projectStatus{
		{
			Project: "proj",
			Agents: []agentStatus{
				{Session: "fleet-proj-dev", Agent: "dev", RelayState: "active", Tasks: 2, HasSession: true},
				{Session: "fleet-proj-ops", Agent: "ops", RelayState: "unregistered", Tasks: -1, HasSession: true},
				{Agent: "ghost", RelayState: "idle", Tasks: 0, HasSession: false},
			},
		},
	}

	out := renderStatus(projects, 2, "")
	if !strings.Contains(out, "2 fleet session(s):") {
		t.Errorf("expected session count header, got: %q", out)
	}
	if !strings.Contains(out, "[proj]") {
		t.Errorf("expected project grouping, got: %q", out)
	}
	if !strings.Contains(out, "fleet-proj-dev  [relay: active · 2 task(s)]") {
		t.Errorf("expected relay state + task count for dev, got: %q", out)
	}
	if !strings.Contains(out, "fleet-proj-ops  [relay: unregistered]") {
		t.Errorf("expected unregistered marker without task count, got: %q", out)
	}
	if !strings.Contains(out, "ghost  [relay: idle · 0 task(s) · no tmux session]") {
		t.Errorf("expected ghost line flagged as having no tmux session, got: %q", out)
	}
}

// Relay unreachable: the warning leads, and the tmux-session view still prints
// with zero invented relay state.
func TestRenderStatusDegraded(t *testing.T) {
	projects := []projectStatus{
		{
			Project: "proj",
			Agents: []agentStatus{
				{Session: "fleet-proj-dev", Agent: "dev", RelayState: "", Tasks: -1, HasSession: true},
			},
		},
	}

	out := renderStatus(projects, 1, "relay unreachable at http://x — showing tmux sessions only")
	if !strings.HasPrefix(out, "  ⚠ relay unreachable") {
		t.Errorf("expected leading warning line, got: %q", out)
	}
	if !strings.Contains(out, "fleet-proj-dev\n") {
		t.Errorf("expected bare session line in degraded mode, got: %q", out)
	}
	if strings.Contains(out, "relay:") || strings.Contains(out, "task") {
		t.Errorf("degraded view must not invent relay state, got: %q", out)
	}
}

// Relay up but a single task count fetch failed: show an explicit unknown.
func TestRenderStatusUnknownTaskCount(t *testing.T) {
	projects := []projectStatus{
		{
			Project: "proj",
			Agents: []agentStatus{
				{Session: "fleet-proj-dev", Agent: "dev", RelayState: "active", Tasks: -1, HasSession: true},
			},
		},
	}

	out := renderStatus(projects, 1, "")
	if !strings.Contains(out, "fleet-proj-dev  [relay: active · tasks: ?]") {
		t.Errorf("expected explicit unknown task count, got: %q", out)
	}
}
