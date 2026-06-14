package coordmgr

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestServeIsReachable runs the real Serve on an ephemeral port against a temp
// FleetDir and asserts coord comes up and answers the health probe through the
// full stack. (The serve goroutine leaks for the test process lifetime — fine.)
func TestServeIsReachable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	go func() { _ = Serve(strconv.Itoa(port)) }()

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	reachable := false
	for i := 0; i < 60; i++ {
		if Reachable(url) {
			reachable = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !reachable {
		t.Fatalf("coord serve did not become reachable at %s", url)
	}
	if _, err := os.Stat(DBPath()); err != nil {
		t.Errorf("coord.db not created at %s: %v", DBPath(), err)
	}
}

func TestEnsureRunningAlreadyReachable(t *testing.T) {
	oldProbe := probe
	probe = func(string) error { return nil }
	defer func() { probe = oldProbe }()

	started := false
	oldStart := startServer
	startServer = func(string) error { started = true; return nil }
	defer func() { startServer = oldStart }()

	if err := EnsureRunning("http://x/mcp", false); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if started {
		t.Error("should not start a child when coord is already reachable")
	}
}

func TestEnsureRunningStartsThenPolls(t *testing.T) {
	calls := 0
	oldProbe := probe
	probe = func(string) error {
		calls++
		if calls <= 1 {
			return fmt.Errorf("down")
		}
		return nil // reachable after the first poll
	}
	defer func() { probe = oldProbe }()

	oldStart := startServer
	started := false
	startServer = func(string) error { started = true; return nil }
	defer func() { startServer = oldStart }()

	if err := EnsureRunning("http://x/mcp", false); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if !started {
		t.Error("expected a start when initially unreachable")
	}
}

func TestEnsureRunningExternalIsNotManaged(t *testing.T) {
	oldProbe := probe
	probe = func(string) error { return fmt.Errorf("down") }
	defer func() { probe = oldProbe }()

	err := EnsureRunning("http://example.com/mcp", true)
	if err == nil || !contains(err.Error(), "not auto-managed") {
		t.Errorf("external unreachable should error, got %v", err)
	}
}

func TestStopIsNoopWithoutPidfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Stop(); err != nil {
		t.Errorf("Stop with no pidfile should be a no-op, got %v", err)
	}
}

func TestStopGuardsOnCoordServe(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(pidPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidPath(), []byte("4242"), 0o644); err != nil {
		t.Fatal(err)
	}

	// pid runs something that is NOT `coord serve` → must not be signalled.
	oldProc, oldKill := procCommand, killProc
	procCommand = func(int) string { return "/usr/bin/some-unrelated-process" }
	signalled := false
	killProc = func(int, syscall.Signal) error { signalled = true; return nil }
	defer func() { procCommand, killProc = oldProc, oldKill }()

	if err := Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if signalled {
		t.Error("ps-guard must not signal a pid that is not running `coord serve`")
	}
	if _, err := os.Stat(pidPath()); !os.IsNotExist(err) {
		t.Error("Stop should remove the pidfile")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
