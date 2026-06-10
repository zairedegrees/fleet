package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/runner"
)

// launchSeams records what the launch pipeline did with cfg.Project.RelayURL:
// its value at save time vs at configure time — the persisted/runtime split
// the --relay-url override must respect.
type launchSeams struct {
	savedURL      string
	configuredURL string
}

// installLaunchSeams stubs every launch side effect (config persistence, tmux
// session creation, iTerm2, background configuration, tmux preflight) so the
// launch pipeline is unit-testable without real sessions.
func installLaunchSeams(t *testing.T) *launchSeams {
	t.Helper()
	s := &launchSeams{}
	origSave, origCreate, origGrid, origConfigure, origList :=
		saveConfigAsLast, createSessions, openITerm2Grid, configureAgentsAsync, listFleetSessions
	t.Cleanup(func() {
		saveConfigAsLast, createSessions, openITerm2Grid, configureAgentsAsync, listFleetSessions =
			origSave, origCreate, origGrid, origConfigure, origList
	})
	saveConfigAsLast = func(cfg *config.FleetConfig) error {
		s.savedURL = cfg.Project.RelayURL
		return nil
	}
	createSessions = func(cfg *config.FleetConfig, claudeBin string) []runner.LaunchResult {
		var results []runner.LaunchResult
		for _, a := range cfg.Agents {
			results = append(results, runner.LaunchResult{Agent: a.Name, Success: true, Action: "created"})
		}
		return results
	}
	openITerm2Grid = func(project string, agents []string) error { return nil }
	configureAgentsAsync = func(cfg *config.FleetConfig) (string, error) {
		s.configuredURL = cfg.Project.RelayURL
		return "", nil
	}
	listFleetSessions = func() ([]string, error) { return nil, nil }
	return s
}

// fakeRelay is a minimal MCP endpoint answering every tools/call with success,
// counting hits — what launch's health check talks to at runtime.
func fakeRelay(t *testing.T, hits *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}]}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setFlagRelayURL(t *testing.T, url string) {
	t.Helper()
	orig := flagRelayURL
	t.Cleanup(func() { flagRelayURL = orig })
	flagRelayURL = url
}

func launchConfig(relayURL string) *config.FleetConfig {
	return &config.FleetConfig{
		Project: config.ProjectConfig{Name: "launch-persist-test", RelayURL: relayURL, Cwd: "/tmp"},
		Claude:  config.ClaudeConfig{Bin: "/bin/echo"},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
	}
}

// --relay-url is a per-invocation override: every runtime relay call of the
// launch must hit it, but the saved config keeps the project's own relay_url —
// `fleet --last --relay-url http://staging/mcp` must not rewrite the project.
func TestLaunchRelayOverrideUsedAtRuntimeButNeverPersisted(t *testing.T) {
	seams := installLaunchSeams(t)
	var originalHits, overrideHits int
	original := fakeRelay(t, &originalHits)
	override := fakeRelay(t, &overrideHits)
	setFlagRelayURL(t, override.URL)

	cfg := launchConfig(original.URL)
	if err := launch(cfg, false); err != nil {
		t.Fatalf("launch failed: %v", err)
	}

	if overrideHits == 0 {
		t.Error("the launch health check must hit the override URL")
	}
	if originalHits != 0 {
		t.Errorf("no runtime call may hit the config URL when overridden, got %d hit(s)", originalHits)
	}
	if seams.savedURL != original.URL {
		t.Errorf("saved config must keep the project's own relay_url %q, got %q", original.URL, seams.savedURL)
	}
	if seams.configuredURL != override.URL {
		t.Errorf("agent configuration must run against the override %q, got %q", override.URL, seams.configuredURL)
	}
	if cfg.Project.RelayURL != original.URL {
		t.Errorf("launch must not leave the override in the config, got %q", cfg.Project.RelayURL)
	}
}

// Without an override the config's own URL drives both runtime and persistence.
func TestLaunchWithoutOverrideUsesConfigURL(t *testing.T) {
	seams := installLaunchSeams(t)
	var hits int
	relaySrv := fakeRelay(t, &hits)
	setFlagRelayURL(t, "")

	cfg := launchConfig(relaySrv.URL)
	if err := launch(cfg, false); err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if hits == 0 {
		t.Error("health check must hit the config relay URL")
	}
	if seams.savedURL != relaySrv.URL || seams.configuredURL != relaySrv.URL {
		t.Errorf("config URL must drive save and configure, got saved=%q configured=%q", seams.savedURL, seams.configuredURL)
	}
}

// Pre-existing behavior: an EMPTY config relay_url is filled with the built-in
// default and persisted as such — never with the flag override.
func TestLaunchEmptyConfigURLPersistsDefaultNotOverride(t *testing.T) {
	seams := installLaunchSeams(t)
	var overrideHits int
	override := fakeRelay(t, &overrideHits)
	setFlagRelayURL(t, override.URL)

	cfg := launchConfig("")
	if err := launch(cfg, false); err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if seams.savedURL != defaultRelayURL {
		t.Errorf("empty config relay_url must persist as the default %q, got %q", defaultRelayURL, seams.savedURL)
	}
	if seams.configuredURL != override.URL {
		t.Errorf("runtime must still use the override %q, got %q", override.URL, seams.configuredURL)
	}
}

// `fleet --last --relay-url ...` end to end: the override drives the relaunch,
// the saved project config keeps its own relay_url.
func TestRunLastRelayOverrideIsNotPersisted(t *testing.T) {
	seams := installLaunchSeams(t)
	var originalHits, overrideHits int
	original := fakeRelay(t, &originalHits)
	override := fakeRelay(t, &overrideHits)
	setFlagRelayURL(t, override.URL)

	origLoad := loadLastConfig
	t.Cleanup(func() { loadLastConfig = origLoad })
	loadLastConfig = func() (*config.FleetConfig, error) { return launchConfig(original.URL), nil }

	if err := runLast(); err != nil {
		t.Fatalf("runLast failed: %v", err)
	}
	if seams.savedURL != original.URL {
		t.Errorf("--last with --relay-url must persist the original %q, got %q", original.URL, seams.savedURL)
	}
	if seams.configuredURL != override.URL {
		t.Errorf("--last runtime must use the override %q, got %q", override.URL, seams.configuredURL)
	}
	if overrideHits == 0 {
		t.Error("health check must hit the override URL")
	}
}
