package main

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/coordmgr"
	"github.com/zairedegrees/fleet/internal/relaymgr"
)

func TestEnsureRelaySetupReachableSkipsConsent(t *testing.T) {
	reachable = func(string) bool { return true }
	consentAsked := false
	askConsent = func(string) bool { consentAsked = true; return true }
	defer func() { reachable = relaymgrReachable; askConsent = defaultAskConsent }()

	if err := ensureRelaySetup("http://localhost:8090/mcp", false, backendDownload); err != nil {
		t.Fatalf("ensureRelaySetup: %v", err)
	}
	if consentAsked {
		t.Error("no consent prompt when a relay is already reachable")
	}
}

func TestEnsureRelaySetupAsksConsentBeforeFirstAcquire(t *testing.T) {
	reachable = func(string) bool { return false }
	asked := false
	askConsent = func(string) bool { asked = true; return false } // user declines
	defer func() { reachable = relaymgrReachable; askConsent = defaultAskConsent }()

	err := ensureRelaySetup("http://localhost:8090/mcp", false, backendDownload)
	if err == nil {
		t.Error("declining consent must abort with an error")
	}
	if !asked {
		t.Error("must ask consent before acquiring the AGPL binary")
	}
}

func TestEnsureRelaySetupEmbeddedSkipsConsentAndDownload(t *testing.T) {
	reachable = func(string) bool { return false }
	askConsent = func(string) bool { t.Error("embedded backend must not prompt for consent"); return false }
	binaryAcquired := false
	ensureRelayBinary = func() (string, error) { binaryAcquired = true; return "", nil }
	skillInstalled, coordStarted := false, false
	installCoordSkill = func() error { skillInstalled = true; return nil }
	ensureCoordRunning = func(string, bool) error { coordStarted = true; return nil }
	defer func() {
		reachable = relaymgrReachable
		askConsent = defaultAskConsent
		ensureRelayBinary = relaymgr.EnsureBinary
		installCoordSkill = coordmgr.InstallSkill
		ensureCoordRunning = coordmgr.EnsureRunning
	}()

	if err := ensureRelaySetup("http://localhost:8090/mcp", false, backendEmbedded); err != nil {
		t.Fatalf("ensureRelaySetup embedded: %v", err)
	}
	if binaryAcquired {
		t.Error("embedded backend must NOT download the AGPL binary")
	}
	if !skillInstalled {
		t.Error("embedded backend must install the /relay skill")
	}
	if !coordStarted {
		t.Error("embedded backend must start coord")
	}
}

func TestExternalRelayBypassesBothBackends(t *testing.T) {
	reachable = func(string) bool { return false }
	defer func() { reachable = relaymgrReachable }()

	err := ensureRelaySetup("http://my-own-relay/mcp", true, backendEmbedded)
	if err == nil || !strings.Contains(err.Error(), "not auto-managed") {
		t.Errorf("external --relay-url must never be auto-managed, got %v", err)
	}
}

func TestCoordBackendResolution(t *testing.T) {
	defer func() { flagRelayBackend = "" }()

	// default
	flagRelayBackend = ""
	t.Setenv("FLEET_RELAY_BACKEND", "")
	if got := coordBackend(nil); got != defaultBackend {
		t.Errorf("default backend = %q, want %q", got, defaultBackend)
	}
	// project config
	if got := coordBackend(&config.FleetConfig{Project: config.ProjectConfig{RelayBackend: backendEmbedded}}); got != backendEmbedded {
		t.Errorf("config backend = %q, want embedded", got)
	}
	// env overrides config
	t.Setenv("FLEET_RELAY_BACKEND", backendDownload)
	if got := coordBackend(&config.FleetConfig{Project: config.ProjectConfig{RelayBackend: backendEmbedded}}); got != backendDownload {
		t.Errorf("env should override config, got %q", got)
	}
	// flag overrides env, and surrounding whitespace is trimmed
	flagRelayBackend = "  embedded  "
	if got := coordBackend(nil); got != backendEmbedded {
		t.Errorf("flag should override env and trim, got %q", got)
	}
}

func TestEnsureRelaySetupUnknownBackendErrors(t *testing.T) {
	reachable = func(string) bool { return false }
	defer func() { reachable = relaymgrReachable }()

	err := ensureRelaySetup("http://localhost:8090/mcp", false, "embeded" /* typo */)
	if err == nil || !strings.Contains(err.Error(), "unknown relay backend") {
		t.Errorf("an unrecognized backend must error (not silently download), got %v", err)
	}
}

func TestStopBackendsNoopWithoutPidfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := stopBackends(); err != nil {
		t.Errorf("stopBackends with no pidfiles should be a no-op, got %v", err)
	}
}
