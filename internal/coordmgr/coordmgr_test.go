package coordmgr

import (
	"fmt"
	"net"
	"os"
	"os/exec"
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

// TestDefaultStartServerSpawnsCoordServe checks the real detached-child spawn
// construction (via the runCmd seam): it must launch `<fleet> coord serve` and
// record the pid, without an actual long-lived child.
func TestDefaultStartServerSpawnsCoordServe(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(pidPath()), 0o755); err != nil {
		t.Fatal(err)
	}

	oldProbe := probe
	probe = func(string) error { return fmt.Errorf("down") } // not reachable -> proceed to spawn
	defer func() { probe = oldProbe }()

	var gotArgs []string
	oldRun := runCmd
	runCmd = func(_ string, args ...string) *exec.Cmd {
		gotArgs = args
		return exec.Command("true") // harmless: Start() succeeds and exits
	}
	defer func() { runCmd = oldRun }()

	if err := defaultStartServer("http://127.0.0.1:18099/mcp"); err != nil {
		t.Fatalf("defaultStartServer: %v", err)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "coord" || gotArgs[1] != "serve" {
		t.Errorf("spawn args = %v, want [coord serve]", gotArgs)
	}
	if _, err := os.Stat(pidPath()); err != nil {
		t.Errorf("pidfile not written: %v", err)
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

func TestInstallSkillWritesEmbedded(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "skills", "relay", "SKILL.md")
	old := skillDest
	skillDest = func() string { return dest }
	defer func() { skillDest = old }()

	if err := InstallSkill(); err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	content := string(b)
	for _, want := range []string{"name: relay", "get_inbox", "talk"} {
		if !contains(content, want) {
			t.Errorf("installed skill missing %q", want)
		}
	}
	if len(content) < 400 {
		t.Errorf("installed skill suspiciously short: %d bytes", len(content))
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
