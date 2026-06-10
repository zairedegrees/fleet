package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

func usageConfig(name string, agents ...config.AgentConfig) *config.FleetConfig {
	return &config.FleetConfig{
		Project: config.ProjectConfig{Name: name},
		Agents:  agents,
	}
}

// The agents total and the polling-vs-idle split come from the saved config —
// auto_talk=true agents poll the relay continuously (burning tokens).
func TestBuildUsageConfigCounts(t *testing.T) {
	fake := &fakeQuerier{}
	installFakeRelay(t, fake)

	cfgs := []*config.FleetConfig{usageConfig("demo",
		config.AgentConfig{Name: "boss", AutoTalk: true},
		config.AgentConfig{Name: "dev", AutoTalk: true},
		config.AgentConfig{Name: "ops"},
	)}
	usages := buildUsage(cfgs, "", defaultRelayURL)
	if len(usages) != 1 {
		t.Fatalf("expected 1 project, got %+v", usages)
	}
	u := usages[0]
	if u.Project != "demo" || u.Agents != 3 || u.Polling != 2 {
		t.Errorf("expected demo with 3 agents, 2 polling, got %+v", u)
	}
}

// Live state comes from the relay: registered agents, how many are active, and
// the total active tasks summed once per unique profile slug.
func TestBuildUsageRelayLiveState(t *testing.T) {
	fake := &fakeQuerier{
		agents: map[string][]relay.Agent{"demo": {
			{Name: "boss", Status: "active"},
			{Name: "dev-1", Status: "idle", ProfileSlug: "dev"},
			{Name: "dev-2", Status: "active", ProfileSlug: "dev"},
		}},
		counts: map[string]int{"demo/boss": 1, "demo/dev": 4},
	}
	installFakeRelay(t, fake)

	usages := buildUsage([]*config.FleetConfig{usageConfig("demo", config.AgentConfig{Name: "boss"})}, "", defaultRelayURL)
	u := usages[0]
	if u.Registered != 3 || u.Active != 2 {
		t.Errorf("expected 3 registered / 2 active from relay, got %+v", u)
	}
	if u.Tasks != 5 {
		t.Errorf("shared slug dev must be counted once (1+4=5), got %d", u.Tasks)
	}
	if len(fake.countCalls) != 2 {
		t.Errorf("expected one count call per unique profile, got %v", fake.countCalls)
	}
}

// Relay down: live numbers are unknown (-1, rendered "?") with an explicit
// warning naming the URL — never silence, never fake zeros.
func TestBuildUsageRelayDownIsExplicit(t *testing.T) {
	fake := &fakeQuerier{listErr: errors.New("connection refused")}
	installFakeRelay(t, fake)

	usages := buildUsage([]*config.FleetConfig{usageConfig("demo", config.AgentConfig{Name: "dev"})}, "", "http://down.example/mcp")
	u := usages[0]
	if u.Registered != -1 || u.Active != -1 || u.Tasks != -1 {
		t.Errorf("relay down must leave live counts unknown (-1), got %+v", u)
	}
	if !strings.Contains(u.RelayWarning, "http://down.example/mcp") {
		t.Errorf("warning must name the relay URL, got %q", u.RelayWarning)
	}
}

// A failed task-count fetch makes the TOTAL unknown — a partial sum is a lie.
func TestBuildUsageTaskFetchFailureIsUnknown(t *testing.T) {
	fake := &fakeQuerier{
		agents:   map[string][]relay.Agent{"demo": {{Name: "dev", Status: "active"}}},
		countErr: errors.New("boom"),
	}
	installFakeRelay(t, fake)

	usages := buildUsage([]*config.FleetConfig{usageConfig("demo", config.AgentConfig{Name: "dev"})}, "", defaultRelayURL)
	u := usages[0]
	if u.Tasks != -1 {
		t.Errorf("failed count fetch must yield unknown tasks (-1), got %d", u.Tasks)
	}
	if u.Registered != 1 {
		t.Errorf("registered count is still known (1), got %d", u.Registered)
	}
}

// Vault bytes mirror what launch would inject: each agent's resolved docs
// summed — a shared doc is pushed once per agent.
func TestBuildUsageVaultBytes(t *testing.T) {
	fake := &fakeQuerier{}
	installFakeRelay(t, fake)

	cwd := t.TempDir()
	shared := filepath.Join(cwd, ".fleet", "vault", "shared")
	if err := os.MkdirAll(shared, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "arch.md"), []byte(strings.Repeat("x", 100)), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := usageConfig("demo", config.AgentConfig{Name: "dev"}, config.AgentConfig{Name: "ops"})
	cfg.Project.Cwd = cwd
	usages := buildUsage([]*config.FleetConfig{cfg}, "", defaultRelayURL)
	u := usages[0]
	if u.VaultBytes != 200 || u.VaultDocs != 2 {
		t.Errorf("expected 100B doc injected once per agent (200B / 2 docs), got bytes=%d docs=%d", u.VaultBytes, u.VaultDocs)
	}
	if u.VaultNote != "" {
		t.Errorf("knowable vault must carry no note, got %q", u.VaultNote)
	}
}

// No cwd in the config means the vault dir is unknowable — that is "?", not 0.
func TestBuildUsageVaultUnknownWithoutCwd(t *testing.T) {
	fake := &fakeQuerier{}
	installFakeRelay(t, fake)

	usages := buildUsage([]*config.FleetConfig{usageConfig("demo", config.AgentConfig{Name: "dev"})}, "", defaultRelayURL)
	u := usages[0]
	if u.VaultBytes != -1 || u.VaultDocs != -1 {
		t.Errorf("unknown vault must be -1, got bytes=%d docs=%d", u.VaultBytes, u.VaultDocs)
	}
	if u.VaultNote == "" {
		t.Error("unknown vault must say why")
	}
}

// A missing vault dir is a true zero: launch would inject nothing.
func TestBuildUsageVaultMissingDirIsZero(t *testing.T) {
	fake := &fakeQuerier{}
	installFakeRelay(t, fake)

	cfg := usageConfig("demo", config.AgentConfig{Name: "dev"})
	cfg.Project.Cwd = t.TempDir()
	usages := buildUsage([]*config.FleetConfig{cfg}, "", defaultRelayURL)
	u := usages[0]
	if u.VaultBytes != 0 || u.VaultDocs != 0 || u.VaultNote != "" {
		t.Errorf("missing vault dir injects nothing — expected honest 0, got %+v", u)
	}
}

// --relay-url beats the project's own saved relay_url, like everywhere else.
func TestBuildUsageFlagOverrideBeatsProjectConfig(t *testing.T) {
	fake := &fakeQuerier{}
	var urls []string
	orig := newStatusClient
	t.Cleanup(func() { newStatusClient = orig })
	newStatusClient = func(url string) relayQuerier {
		urls = append(urls, url)
		return fake
	}

	cfg := usageConfig("demo", config.AgentConfig{Name: "dev"})
	cfg.Project.RelayURL = "http://own.example/mcp"
	buildUsage([]*config.FleetConfig{cfg}, "http://override/mcp", "http://fallback/mcp")

	if len(urls) != 1 || urls[0] != "http://override/mcp" {
		t.Errorf("--relay-url must beat the project config URL, got %v", urls)
	}
}

// Without an override, each project resolves its own relay_url with the
// fallback chain — same rule as --status.
func TestBuildUsagePerProjectRelayURL(t *testing.T) {
	fake := &fakeQuerier{}
	var urls []string
	orig := newStatusClient
	t.Cleanup(func() { newStatusClient = orig })
	newStatusClient = func(url string) relayQuerier {
		urls = append(urls, url)
		return fake
	}

	a := usageConfig("a", config.AgentConfig{Name: "dev"})
	a.Project.RelayURL = "http://a.example/mcp"
	b := usageConfig("b", config.AgentConfig{Name: "dev"})
	buildUsage([]*config.FleetConfig{a, b}, "", "http://fallback/mcp")

	want := map[string]bool{"http://a.example/mcp": true, "http://fallback/mcp": true}
	for _, u := range urls {
		delete(want, u)
	}
	if len(want) != 0 {
		t.Errorf("expected a client per project relay URL, missing %v (got %v)", want, urls)
	}
}

// renderUsage is pure and every number states its source; the relay line and
// the vault line carry explicit [config]/[relay] provenance.
func TestRenderUsageFullView(t *testing.T) {
	out := renderUsage([]projectUsage{{
		Project:    "demo",
		RelayURL:   "http://localhost:8090/mcp",
		Agents:     3,
		Polling:    1,
		Registered: 3,
		Active:     2,
		Tasks:      5,
		VaultBytes: 12 * 1024,
		VaultDocs:  4,
	}})
	for _, want := range []string{
		"[demo]",
		"http://localhost:8090/mcp",
		"3 declared",
		"1 polling (auto_talk)",
		"2 idle",
		"[config]",
		"3 registered",
		"2 active",
		"5 active task(s)",
		"[relay]",
		"12.0 KB",
		"4 doc injection(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

// Unknowns render as "?" with the reason — never as 0, never silently.
func TestRenderUsageUnknowns(t *testing.T) {
	out := renderUsage([]projectUsage{{
		Project:      "demo",
		RelayURL:     "http://down.example/mcp",
		Agents:       2,
		Polling:      0,
		Registered:   -1,
		Active:       -1,
		Tasks:        -1,
		RelayWarning: "relay unavailable at http://down.example/mcp (connection refused)",
		VaultBytes:   -1,
		VaultDocs:    -1,
		VaultNote:    "project cwd not set in config",
	}})
	if !strings.Contains(out, "⚠ relay unavailable at http://down.example/mcp") {
		t.Errorf("relay-down must be an explicit warning, got:\n%s", out)
	}
	if !strings.Contains(out, "live (relay):    ?") {
		t.Errorf("unknown live state must render '?', got:\n%s", out)
	}
	if !strings.Contains(out, "vault (config):  ?") || !strings.Contains(out, "cwd not set") {
		t.Errorf("unknown vault must render '?' with the reason, got:\n%s", out)
	}
}

// Relay up but a task total that could not be fetched: registered/active are
// real, the task total is an honest "?".
func TestRenderUsageUnknownTasksOnly(t *testing.T) {
	out := renderUsage([]projectUsage{{
		Project: "demo", RelayURL: "http://x/mcp",
		Agents: 1, Registered: 1, Active: 1, Tasks: -1,
		VaultBytes: 0, VaultDocs: 0,
	}})
	if !strings.Contains(out, "tasks: ?") {
		t.Errorf("unfetched task total must render 'tasks: ?', got:\n%s", out)
	}
}
