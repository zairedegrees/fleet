package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

func testCfg(dir string) *config.FleetConfig {
	return &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: dir},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
	}
}

// fakeRegistrar satisfies relayRegistrar without a relay; err is returned by
// every call.
type fakeRegistrar struct {
	err            error
	notifyChannels []string
}

func (f *fakeRegistrar) EnsureProfile(name, role, project string) error          { return f.err }
func (f *fakeRegistrar) RegisterAgentFull(r relay.AgentRegistration) error       { return f.err }
func (f *fakeRegistrar) PushVaultDoc(project, path string, content []byte) error { return f.err }
func (f *fakeRegistrar) RegisterNotifyChannel(project, agent, target string) error {
	f.notifyChannels = append(f.notifyChannels, agent+"="+target)
	return f.err
}

// A mutant that ignores cfg.Project.RelayURL and registers against the default
// relay must not survive: launch registration has to hit the config's URL.
func TestConfigureAgentsAsyncRegistersAgainstConfigRelayURL(t *testing.T) {
	stubExec(t)
	t.Setenv("HOME", t.TempDir())
	srv, calls := captureRelay(t)
	cfg := testCfg(t.TempDir())
	cfg.Project.RelayURL = srv.URL

	if _, err := ConfigureAgentsAsync(cfg); err != nil {
		t.Fatalf("ConfigureAgentsAsync failed: %v", err)
	}
	if got := len(callsFor(*calls, "register_agent")); got != 1 {
		t.Errorf("expected 1 register_agent call at the config relay URL, got %d", got)
	}
}

// The registrar targets the config URL verbatim, and the default ONLY when the
// config field is empty — pinned via the construction seam so the empty case
// never touches a live relay on the default URL.
func TestConfigureAgentsAsyncRelayURLResolution(t *testing.T) {
	stubExec(t)
	t.Setenv("HOME", t.TempDir())
	var gotURL string
	orig := newRegistrar
	newRegistrar = func(url string) relayRegistrar {
		gotURL = url
		return &fakeRegistrar{}
	}
	t.Cleanup(func() { newRegistrar = orig })

	cfg := testCfg(t.TempDir())
	cfg.Project.RelayURL = "http://relay.example:7777/mcp"
	if _, err := ConfigureAgentsAsync(cfg); err != nil {
		t.Fatalf("ConfigureAgentsAsync failed: %v", err)
	}
	if gotURL != "http://relay.example:7777/mcp" {
		t.Errorf("registrar must target the config URL, got %q", gotURL)
	}

	cfg.Project.RelayURL = ""
	if _, err := ConfigureAgentsAsync(cfg); err != nil {
		t.Fatalf("ConfigureAgentsAsync failed: %v", err)
	}
	if gotURL != config.DefaultRelayURL {
		t.Errorf("empty config must fall back to %s, got %q", config.DefaultRelayURL, gotURL)
	}
}

// The configure step must report where it logs and that it actually spawned —
// instead of fire-and-forgetting with a void signature.
func TestConfigureAgentsReturnsLogPathAndSpawns(t *testing.T) {
	stubExec(t) // every tmux has-session succeeds
	dir := t.TempDir()
	spawned := ""
	logPath, err := configureAgents(testCfg(dir), dir, func(p string) error { spawned = p; return nil }, &fakeRegistrar{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logPath == "" {
		t.Error("expected a non-empty log path")
	}
	if spawned == "" {
		t.Error("expected the configure script to be spawned")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "configure-agents.sh")); statErr != nil {
		t.Errorf("configure script not written under fleetDir: %v", statErr)
	}
}

// A spawn failure (e.g. fork error) must surface, not be swallowed.
func TestConfigureAgentsSurfacesSpawnError(t *testing.T) {
	stubExec(t)
	dir := t.TempDir()
	_, err := configureAgents(testCfg(dir), dir, func(p string) error { return errors.New("fork failed") }, &fakeRegistrar{})
	if err == nil {
		t.Fatal("expected spawn failure to surface, got nil")
	}
	if !strings.Contains(err.Error(), "fork failed") {
		t.Errorf("error should wrap the spawn failure, got: %v", err)
	}
}

// If setup fails (log dir cannot be created) we must error and never spawn.
func TestConfigureAgentsErrorsBeforeSpawnOnSetupFailure(t *testing.T) {
	stubExec(t)
	dir := t.TempDir()
	// Make "logs" a FILE so MkdirAll(dir/logs) fails.
	if err := os.WriteFile(filepath.Join(dir, "logs"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err := configureAgents(testCfg(dir), dir, func(p string) error { called = true; return nil }, &fakeRegistrar{})
	if err == nil {
		t.Fatal("expected an error when the log dir cannot be created, got nil")
	}
	if called {
		t.Error("spawn must not run when setup failed")
	}
}

// A relay registration failure must surface in the returned error, but the
// detached pane-configure script must still spawn — rename/color/send-keys
// are independent of the HTTP registration.
func TestConfigureAgentsSurfacesRelayErrorButStillSpawns(t *testing.T) {
	stubExec(t)
	dir := t.TempDir()
	spawned := false
	_, err := configureAgents(testCfg(dir), dir, func(p string) error { spawned = true; return nil }, &fakeRegistrar{err: errors.New("relay down")})
	if err == nil {
		t.Fatal("expected the registration failure to surface, got nil")
	}
	if !strings.Contains(err.Error(), "relay down") {
		t.Errorf("error should wrap the relay failure, got: %v", err)
	}
	if !spawned {
		t.Error("registration failure must not prevent the pane-configure spawn")
	}
}
