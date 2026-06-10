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
	err error
}

func (f *fakeRegistrar) EnsureProfile(name, role, project string) error          { return f.err }
func (f *fakeRegistrar) RegisterAgentFull(r relay.AgentRegistration) error       { return f.err }
func (f *fakeRegistrar) PushVaultDoc(project, path string, content []byte) error { return f.err }

// The configure step must report where it logs and that it actually spawned —
// instead of fire-and-forgetting with a void signature.
func TestConfigureAgentsReturnsLogPathAndSpawns(t *testing.T) {
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
