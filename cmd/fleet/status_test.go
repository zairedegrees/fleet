package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestDeriveOpState(t *testing.T) {
	cases := []struct {
		name       string
		relayState string
		tasks      int
		want       string
	}{
		{"active no tasks is idle", "active", 0, "idle"},
		{"active with tasks is working", "active", 2, "working"},
		{"active unknown tasks is registered", "active", -1, "registered"},
		{"unregistered passthrough", "unregistered", -1, "unregistered"},
		{"unknown project passthrough", "?", -1, "?"},
		{"inactive passthrough", "inactive", 0, "inactive"},
		{"empty passthrough", "", -1, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveOpState(c.relayState, c.tasks); got != c.want {
				t.Errorf("deriveOpState(%q,%d) = %q, want %q", c.relayState, c.tasks, got, c.want)
			}
		})
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	rfc := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	cases := []struct {
		name     string
		lastSeen string
		want     string
	}{
		{"seconds", rfc(12 * time.Second), "12s ago"},
		{"minutes", rfc(3 * time.Minute), "3m ago"},
		{"hours", rfc(2 * time.Hour), "2h ago"},
		{"days", rfc(50 * time.Hour), "2d ago"},
		{"empty", "", ""},
		{"unparsable", "not-a-time", ""},
		{"future clock skew", now.Add(30 * time.Second).Format(time.RFC3339), "just now"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := relativeTime(c.lastSeen, now); got != c.want {
				t.Errorf("relativeTime(%q) = %q, want %q", c.lastSeen, got, c.want)
			}
		})
	}
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

// P1-A: dot-projects are sanitized one-way for tmux (acme.io → acme-io),
// so the resolver must map the session back to the REAL project name — that is
// the name the relay was registered with.
func TestResolveSessionDotProject(t *testing.T) {
	project, agent, known := resolveSession("fleet-acme-io-dev", []string{"acme.io"})
	if !known || project != "acme.io" || agent != "dev" {
		t.Errorf("expected acme.io/dev (known), got %s/%s known=%v", project, agent, known)
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

	projects, warning := buildStatus([]string{"fleet-demo-ux-designer"}, projectConfigs("demo"), defaultRelayURL, "")
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
		agents: map[string][]relay.Agent{"acme.io": {{Name: "dev", Status: "active"}}},
		counts: map[string]int{"acme.io/dev": 2},
	}
	installFakeRelay(t, fake)

	projects, _ := buildStatus([]string{"fleet-acme-io-dev"}, projectConfigs("acme.io"), defaultRelayURL, "")
	if len(projects) != 1 || projects[0].Project != "acme.io" {
		t.Fatalf("expected project group acme.io, got %+v", projects)
	}
	a := projects[0].Agents[0]
	if a.Agent != "dev" || a.RelayState != "active" || a.Tasks != 2 {
		t.Errorf("expected dev with relay state + count, got %+v", a)
	}
	if len(fake.listCalls) == 0 || fake.listCalls[0] != "acme.io" {
		t.Errorf("relay must be queried with the real project name, got %v", fake.listCalls)
	}
}

// P1-A: a session matching no known project gets an honest "?" — never
// "unregistered" — and the relay is never queried with a guessed name.
func TestBuildStatusUnknownSessionIsHonest(t *testing.T) {
	fake := &fakeQuerier{}
	installFakeRelay(t, fake)

	projects, _ := buildStatus([]string{"fleet-mystery-agent"}, projectConfigs("demo"), defaultRelayURL, "")
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

// P2: zero tmux sessions must still surface relay-registered ghosts — the old
// early return made them invisible.
func TestBuildStatusRendersGhostsWithZeroSessions(t *testing.T) {
	fake := &fakeQuerier{
		agents: map[string][]relay.Agent{"demo": {{Name: "ghost", Status: "idle"}}},
		counts: map[string]int{"demo/ghost": 0},
	}
	installFakeRelay(t, fake)

	projects, warning := buildStatus(nil, projectConfigs("demo"), defaultRelayURL, "")
	if warning != "" {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if len(projects) != 1 || projects[0].Project != "demo" {
		t.Fatalf("expected ghost project demo, got %+v", projects)
	}
	g := projects[0].Agents[0]
	if g.Agent != "ghost" || g.HasSession || g.RelayState != "idle" || g.Tasks != 0 {
		t.Errorf("expected relay-only ghost with state + count, got %+v", g)
	}
}

// P2: zero sessions + relay down — no invented projects, but the degraded
// warning must surface so the user knows ghosts may be hidden.
func TestBuildStatusZeroSessionsRelayDownWarns(t *testing.T) {
	fake := &fakeQuerier{listErr: errors.New("connection refused")}
	installFakeRelay(t, fake)

	projects, warning := buildStatus(nil, projectConfigs("demo"), defaultRelayURL, "")
	if len(projects) != 0 {
		t.Errorf("expected no projects when relay is down and no sessions, got %+v", projects)
	}
	if !strings.Contains(warning, "relay unavailable") {
		t.Errorf("expected degraded warning, got %q", warning)
	}
}

// P2: each project's relay URL comes from its OWN saved config, falling back
// to the default — one project's relay must not answer for another's.
func TestRelayURLForPrefersProjectConfig(t *testing.T) {
	configs := []*config.FleetConfig{
		{Project: config.ProjectConfig{Name: "a", RelayURL: "http://a.example/mcp"}},
		{Project: config.ProjectConfig{Name: "b"}},
	}
	if got := relayURLFor("a", configs, "http://fallback/mcp"); got != "http://a.example/mcp" {
		t.Errorf("project a must use its own relay URL, got %q", got)
	}
	if got := relayURLFor("b", configs, "http://fallback/mcp"); got != "http://fallback/mcp" {
		t.Errorf("project b has no URL, must fall back, got %q", got)
	}
	if got := relayURLFor("zzz", configs, "http://fallback/mcp"); got != "http://fallback/mcp" {
		t.Errorf("unknown project must fall back, got %q", got)
	}
}

func TestBuildStatusUsesPerProjectRelayURL(t *testing.T) {
	fake := &fakeQuerier{}
	var urls []string
	orig := newStatusClient
	t.Cleanup(func() { newStatusClient = orig })
	newStatusClient = func(url string) relayQuerier {
		urls = append(urls, url)
		return fake
	}

	configs := []*config.FleetConfig{
		{Project: config.ProjectConfig{Name: "a", RelayURL: "http://a.example/mcp"}},
		{Project: config.ProjectConfig{Name: "b"}},
	}
	buildStatus([]string{"fleet-a-dev", "fleet-b-dev"}, configs, "http://fallback/mcp", "")

	want := map[string]bool{"http://a.example/mcp": true, "http://fallback/mcp": true}
	for _, u := range urls {
		delete(want, u)
	}
	if len(want) != 0 {
		t.Errorf("expected a client per project relay URL, missing %v (got %v)", want, urls)
	}
}

// --relay-url overrides EVERY project's relay resolution — even a project
// whose saved config carries its own relay_url.
func TestBuildStatusFlagOverrideBeatsProjectConfig(t *testing.T) {
	fake := &fakeQuerier{}
	var urls []string
	orig := newStatusClient
	t.Cleanup(func() { newStatusClient = orig })
	newStatusClient = func(url string) relayQuerier {
		urls = append(urls, url)
		return fake
	}

	configs := []*config.FleetConfig{
		{Project: config.ProjectConfig{Name: "a", RelayURL: "http://a.example/mcp"}},
	}
	buildStatus([]string{"fleet-a-dev"}, configs, "http://fallback/mcp", "http://override/mcp")

	if len(urls) == 0 {
		t.Fatal("expected a relay client to be created")
	}
	for _, u := range urls {
		if u != "http://override/mcp" {
			t.Errorf("--relay-url must beat the project config URL, got client for %q", u)
		}
	}
}

// P2: a hanging relay must not stall --status for the default client's 10s —
// status queries get a snappy ~2s timeout.
func TestStatusClientTimesOutFast(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	start := time.Now()
	_, err := newStatusClient(server.URL).ListAgents("proj")
	if err == nil {
		t.Fatal("expected a timeout error from a hanging relay, got nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("status client must time out fast, took %v", elapsed)
	}
}

// P2: agents sharing a profile slug must share ONE task-count fetch — same
// count for both, no duplicate relay calls.
func TestFetchTaskCountsMemoizesSharedProfiles(t *testing.T) {
	fake := &fakeQuerier{counts: map[string]int{"proj/dev": 3}}
	agents := []relay.Agent{
		{Name: "dev-1", ProfileSlug: "dev"},
		{Name: "dev-2", ProfileSlug: "dev"},
		{Name: "ops"},
	}

	tasks := fetchTaskCounts(fake, "proj", agents)
	if tasks["dev-1"] != 3 || tasks["dev-2"] != 3 {
		t.Errorf("agents sharing a slug must share the count, got %v", tasks)
	}
	if len(fake.countCalls) != 2 {
		t.Errorf("expected 1 call per unique profile (dev, ops), got %v", fake.countCalls)
	}
}

// A failed count fetch leaves no entry (renders "tasks: ?") and is not
// retried for every agent sharing the slug.
func TestFetchTaskCountsFailureIsNotFakedOrRetried(t *testing.T) {
	fake := &fakeQuerier{countErr: errors.New("boom")}
	agents := []relay.Agent{
		{Name: "dev-1", ProfileSlug: "dev"},
		{Name: "dev-2", ProfileSlug: "dev"},
	}

	tasks := fetchTaskCounts(fake, "proj", agents)
	if len(tasks) != 0 {
		t.Errorf("failed fetch must not fake a count, got %v", tasks)
	}
	if len(fake.countCalls) != 1 {
		t.Errorf("failed fetch must not be retried per agent, got %v", fake.countCalls)
	}
}

// The "?" state renders as an honest unknown without a task count.
// renderRef is a fixed "now" so seen-ago segments are deterministic in tests.
var renderRef = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

func TestRenderStatusUnknownRelayState(t *testing.T) {
	projects := []projectStatus{
		{
			Project: "mystery",
			Agents: []agentStatus{
				{Session: "fleet-mystery-agent", Agent: "agent", RelayState: relayStateUnknown, Tasks: -1, HasSession: true},
			},
		},
	}

	out := renderStatus(projects, 1, "", renderRef)
	if !strings.Contains(out, "agent  [?]") {
		t.Errorf("expected honest unknown state with short name, got: %q", out)
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
		{Name: "dev", Status: "active", LastSeen: "2026-06-15T02:17:50Z"},
		{Name: "ghost", Status: "idle"},
	}
	tasks := map[string]int{"dev": 2, "ghost": 0}
	posture := map[string]bool{"dev": true, "ops": false}

	agents := mergeAgents("proj", sessions, relayAgents, tasks, posture, true)
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
	if !dev.AutoTalk {
		t.Errorf("dev posture auto_talk should be threaded through, got %+v", dev)
	}
	if dev.LastSeen != "2026-06-15T02:17:50Z" {
		t.Errorf("dev should carry last_seen, got %q", dev.LastSeen)
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
	agents := mergeAgents("proj", []string{"fleet-proj-dev"}, nil, nil, nil, false)
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
		[]relay.Agent{{Name: "dev", Status: "active"}}, map[string]int{}, nil, true)
	if agents[0].Tasks != -1 {
		t.Errorf("missing task count must stay -1, got %d", agents[0].Tasks)
	}
}

func TestRenderStatusFullView(t *testing.T) {
	seen := renderRef.Add(-2 * time.Minute).Format(time.RFC3339)
	projects := []projectStatus{
		{
			Project: "proj",
			Agents: []agentStatus{
				{Session: "fleet-proj-dev", Agent: "dev", RelayState: "active", Tasks: 2, HasSession: true, AutoTalk: false, LastSeen: seen},
				{Session: "fleet-proj-ops", Agent: "ops", RelayState: "unregistered", Tasks: -1, HasSession: true},
				{Agent: "watcher", RelayState: "active", Tasks: 0, HasSession: false, AutoTalk: true, LastSeen: seen},
			},
		},
	}

	out := renderStatus(projects, 2, "", renderRef)
	if !strings.Contains(out, "2 fleet session(s):") {
		t.Errorf("expected session count header, got: %q", out)
	}
	if !strings.Contains(out, "[proj]") {
		t.Errorf("expected project grouping, got: %q", out)
	}
	if !strings.Contains(out, "dev  [working · on-demand · 2 task(s) · seen 2m ago]") {
		t.Errorf("expected working line for dev, got: %q", out)
	}
	if !strings.Contains(out, "ops  [unregistered]") {
		t.Errorf("expected unregistered marker without posture/tasks, got: %q", out)
	}
	if !strings.Contains(out, "watcher  [idle · auto-talk · seen 2m ago · no tmux session]") {
		t.Errorf("expected idle ghost with posture + seen + no-session flag, got: %q", out)
	}
	// idle present → legend must render once.
	if !strings.Contains(out, "idle = registered, in standby") {
		t.Errorf("expected idle legend, got: %q", out)
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

	out := renderStatus(projects, 1, "relay unreachable at http://x — showing tmux sessions only", renderRef)
	if !strings.HasPrefix(out, "  ⚠ relay unreachable") {
		t.Errorf("expected leading warning line, got: %q", out)
	}
	if !strings.Contains(out, "      dev\n") {
		t.Errorf("expected bare short-name line in degraded mode, got: %q", out)
	}
	if strings.Contains(out, "relay:") || strings.Contains(out, "task") || strings.Contains(out, "idle") {
		t.Errorf("degraded view must not invent state, got: %q", out)
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

	out := renderStatus(projects, 1, "", renderRef)
	if !strings.Contains(out, "dev  [registered · on-demand · tasks: ?]") {
		t.Errorf("expected registered with explicit unknown task count, got: %q", out)
	}
}
