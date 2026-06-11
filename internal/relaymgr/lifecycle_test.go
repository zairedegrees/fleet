package relaymgr

import "testing"

func TestEnsureRunningReachableIsNoop(t *testing.T) {
	probe = func(string) error { return nil }
	started := false
	startServer = func(bin, url string) error { started = true; return nil }
	defer func() { probe = defaultProbe; startServer = defaultStartServer }()

	if err := EnsureRunning("http://localhost:8090/mcp", false); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if started {
		t.Error("must not start a server when one is already reachable")
	}
}

func TestEnsureRunningExternalUrlDoesNotAutoStart(t *testing.T) {
	probe = func(string) error { return errUnreachable }
	startServer = func(bin, url string) error { t.Fatal("must not start for external url"); return nil }
	defer func() { probe = defaultProbe; startServer = defaultStartServer }()

	if err := EnsureRunning("http://remote:8090/mcp", true); err == nil {
		t.Error("expected error for unreachable external relay")
	}
}

func TestEnsureRunningDefaultAcquiresAndStarts(t *testing.T) {
	calls := 0
	probe = func(string) error { // unreachable first, reachable after start
		calls++
		if calls == 1 {
			return errUnreachable
		}
		return nil
	}
	ensureBin = func() (string, error) { return "/tmp/agent-relay", nil }
	started := false
	startServer = func(bin, url string) error { started = true; return nil }
	defer func() {
		probe = defaultProbe
		ensureBin = EnsureBinary
		startServer = defaultStartServer
	}()

	if err := EnsureRunning("http://localhost:8090/mcp", false); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if !started {
		t.Error("expected the managed server to be started")
	}
}

func TestPortFromURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://localhost:8090/mcp", "8090"},
		{"http://localhost:9999/mcp", "9999"},
		{"http://host/mcp", "8090"},
		{"https://localhost:8443/mcp", "8443"},
		{"http://localhost:8090", "8090"},
		{"http://host", "8090"},
	}
	for _, c := range cases {
		if got := portFromURL(c.in); got != c.want {
			t.Errorf("portFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
