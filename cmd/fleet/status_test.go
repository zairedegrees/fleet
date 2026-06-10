package main

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

// fakeQuerier stands in for the relay client so the status pipeline is
// testable without a relay.
type fakeQuerier struct {
	agents     map[string][]relay.Agent
	counts     map[string]int // key: project + "/" + profile
	listErr    error
	countErr   error
	listCalls  []string
	countCalls []string
}

func (f *fakeQuerier) ListAgents(project string) ([]relay.Agent, error) {
	f.listCalls = append(f.listCalls, project)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.agents[project], nil
}

func (f *fakeQuerier) CountActiveTasks(project, profile string) (int, error) {
	key := project + "/" + profile
	f.countCalls = append(f.countCalls, key)
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.counts[key], nil
}

func installFakeRelay(t *testing.T, f *fakeQuerier) {
	t.Helper()
	orig := newStatusClient
	t.Cleanup(func() { newStatusClient = orig })
	newStatusClient = func(url string) relayQuerier { return f }
}

func projectConfigs(names ...string) []*config.FleetConfig {
	var cfgs []*config.FleetConfig
	for _, n := range names {
		cfgs = append(cfgs, &config.FleetConfig{Project: config.ProjectConfig{Name: n}})
	}
	return cfgs
}

// P1-A: agent names contain dashes (ux-designer ships in 3/5 presets), so a
// session must resolve against KNOWN project names — never by guessing on the
// last dash.
func TestResolveSessionDashNamedAgent(t *testing.T) {
	project, agent, known := resolveSession("fleet-demo-ux-designer", []string{"demo"})
	if !known || project != "demo" || agent != "ux-designer" {
		t.Errorf("expected demo/ux-designer (known), got %s/%s known=%v", project, agent, known)
	}
}

// P1-A: dot-projects are sanitized one-way for tmux (v1stud.io → v1stud-io),
// so the resolver must map the session back to the REAL project name — that is
// the name the relay was registered with.
func TestResolveSessionDotProject(t *testing.T) {
	project, agent, known := resolveSession("fleet-v1stud-io-dev", []string{"v1stud.io"})
	if !known || project != "v1stud.io" || agent != "dev" {
		t.Errorf("expected v1stud.io/dev (known), got %s/%s known=%v", project, agent, known)
	}
}

func TestResolveSessionLongestPrefixWins(t *testing.T) {
	project, agent, known := resolveSession("fleet-demo-ux-designer", []string{"demo", "demo-ux"})
	if !known || project != "demo-ux" || agent != "designer" {
		t.Errorf("expected demo-ux/designer (known), got %s/%s known=%v", project, agent, known)
	}
}

func TestResolveSessionUnknownProject(t *testing.T) {
	project, agent, known := resolveSession("fleet-mystery-agent", []string{"demo"})
	if known {
		t.Fatalf("unknown session must not claim a known project, got %s/%s", project, agent)
	}
	if project != "mystery" || agent != "agent" {
		t.Errorf("fallback should guess on the last dash, got %s/%s", project, agent)
	}
}

// P1-A end-to-end: session fleet-demo-ux-designer with known project "demo"
// must render ONE agent ux-designer with its relay state — not a bogus
// project demo-ux with an "unregistered" agent plus a ghost (double lie).
func TestBuildStatusDashAgentResolvesAgainstKnownProject(t *testing.T) {
	fake := &fakeQuerier{
		agents: map[string][]relay.Agent{"demo": {{Name: "ux-designer", Status: "active"}}},
		counts: map[string]int{"demo/ux-designer": 1},
	}
	installFakeRelay(t, fake)

	projects, warning := buildStatus([]string{"fleet-demo-ux-designer"}, projectConfigs("demo"), defaultRelayURL)
	if warning != "" {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if len(projects) != 1 || projects[0].Project != "demo" {
		t.Fatalf("expected single project demo, got %+v", projects)
	}
	agents := projects[0].Agents
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent (no ghost), got %+v", agents)
	}
	a := agents[0]
	if a.Agent != "ux-designer" || !a.HasSession || a.RelayState != "active" || a.Tasks != 1 {
		t.Errorf("expected ux-designer with session + relay state + count, got %+v", a)
	}
	for _, p := range fake.listCalls {
		if p != "demo" {
			t.Errorf("relay must only be queried with the real project name, got %q", p)
		}
	}
}

// P1-A: relay queries for a dot-project must use the REAL name (registration
// uses the raw project name), and the group label stays the real name too.
func TestBuildStatusDotProjectQueriesRealName(t *testing.T) {
	fake := &fakeQuerier{
		agents: map[string][]relay.Agent{"v1stud.io": {{Name: "dev", Status: "active"}}},
		counts: map[string]int{"v1stud.io/dev": 2},
	}
	installFakeRelay(t, fake)

	projects, _ := buildStatus([]string{"fleet-v1stud-io-dev"}, projectConfigs("v1stud.io"), defaultRelayURL)
	if len(projects) != 1 || projects[0].Project != "v1stud.io" {
		t.Fatalf("expected project group v1stud.io, got %+v", projects)
	}
	a := projects[0].Agents[0]
	if a.Agent != "dev" || a.RelayState != "active" || a.Tasks != 2 {
		t.Errorf("expected dev with relay state + count, got %+v", a)
	}
	if len(fake.listCalls) == 0 || fake.listCalls[0] != "v1stud.io" {
		t.Errorf("relay must be queried with the real project name, got %v", fake.listCalls)
	}
}

// P1-A: a session matching no known project gets an honest "?" — never
// "unregistered" — and the relay is never queried with a guessed name.
func TestBuildStatusUnknownSessionIsHonest(t *testing.T) {
	fake := &fakeQuerier{}
	installFakeRelay(t, fake)

	projects, _ := buildStatus([]string{"fleet-mystery-agent"}, projectConfigs("demo"), defaultRelayURL)
	var found *agentStatus
	for i := range projects {
		for j := range projects[i].Agents {
			if projects[i].Agents[j].Session == "fleet-mystery-agent" {
				found = &projects[i].Agents[j]
			}
		}
	}
	if found == nil {
		t.Fatalf("unknown session must still be listed, got %+v", projects)
	}
	if found.RelayState != relayStateUnknown {
		t.Errorf("unknown session must show %q, got %q", relayStateUnknown, found.RelayState)
	}
	if found.Tasks != -1 {
		t.Errorf("unknown session has no task count, expected -1, got %d", found.Tasks)
	}
	for _, p := range fake.listCalls {
		if p == "mystery" {
			t.Errorf("relay must not be queried with a guessed project name, got %v", fake.listCalls)
		}
	}
}

// The "?" state renders as an honest unknown without a task count.
func TestRenderStatusUnknownRelayState(t *testing.T) {
	projects := []projectStatus{
		{
			Project: "mystery",
			Agents: []agentStatus{
				{Session: "fleet-mystery-agent", Agent: "agent", RelayState: relayStateUnknown, Tasks: -1, HasSession: true},
			},
		},
	}

	out := renderStatus(projects, 1, "")
	if !strings.Contains(out, "fleet-mystery-agent  [relay: ?]") {
		t.Errorf("expected honest unknown relay state, got: %q", out)
	}
	if strings.Contains(out, "unregistered") || strings.Contains(out, "task") {
		t.Errorf("unknown state must not assert registration or tasks, got: %q", out)
	}
}

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
