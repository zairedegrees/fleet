package main

import "testing"

func TestEnsureRelaySetupReachableSkipsConsent(t *testing.T) {
	reachable = func(string) bool { return true }
	consentAsked := false
	askConsent = func(string) bool { consentAsked = true; return true }
	defer func() { reachable = relaymgrReachable; askConsent = defaultAskConsent }()

	if err := ensureRelaySetup("http://localhost:8090/mcp", false); err != nil {
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

	err := ensureRelaySetup("http://localhost:8090/mcp", false)
	if err == nil {
		t.Error("declining consent must abort with an error")
	}
	if !asked {
		t.Error("must ask consent before acquiring the AGPL binary")
	}
}
