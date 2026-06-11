package relaymgr

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

// errUnreachable is a test sentinel for probe stubs (production builds its own
// errors with context).
var errUnreachable = errors.New("relay unreachable")

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

func TestStopOnlySignalsAgentRelay(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(config.FleetDir(), 0755); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(config.FleetDir(), "relay.pid")

	cases := []struct {
		name     string
		cmdline  string
		wantKill bool
	}{
		{"is agent-relay", "/Users/x/.fleet/bin/agent-relay serve", true},
		{"recycled pid", "vim notes.txt", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			os.WriteFile(pidFile, []byte("4242"), 0644)
			procCommand = func(int) string { return c.cmdline }
			killed := -1
			killProc = func(pid int, _ syscall.Signal) error { killed = pid; return nil }
			defer func() { procCommand = defaultProcCommand; killProc = syscall.Kill }()

			if err := Stop(); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if c.wantKill && killed != 4242 {
				t.Errorf("expected SIGTERM to pid 4242, got %d", killed)
			}
			if !c.wantKill && killed != -1 {
				t.Errorf("must not signal a recycled pid, killed %d", killed)
			}
			if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
				t.Error("pidfile should be removed after Stop")
			}
		})
	}
}
