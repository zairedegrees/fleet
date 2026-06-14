package doctor

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

// swapExecCommand replaces the execCommand seam with a fake that records each
// probe's argv and runs mk() instead; the real seam is restored on cleanup.
func swapExecCommand(t *testing.T, mk func() *exec.Cmd) *[][]string {
	t.Helper()
	var calls [][]string
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		return mk()
	}
	t.Cleanup(func() { execCommand = orig })
	return &calls
}

// failingCmd returns a command whose Output() always errors, simulating a
// probe binary missing from PATH.
func failingCmd() *exec.Cmd {
	return exec.Command("fleet-doctor-no-such-binary")
}

// healthyRelay returns an httptest server that answers the MCP tools/call the
// way the real wrai.th relay does (a JSON-RPC envelope with a text content block).
func healthyRelay() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"count\":0,\"orgs\":[]}"}]}}`)
	}))
}

func TestCheckRelayOK(t *testing.T) {
	srv := healthyRelay()
	defer srv.Close()

	c := checkRelay(srv.URL, "download")
	if c.Status != "ok" {
		t.Fatalf("expected status ok, got %q (detail: %s)", c.Status, c.Detail)
	}
}

func TestCheckRelayUnreachable(t *testing.T) {
	srv := healthyRelay()
	url := srv.URL
	srv.Close() // nothing listening anymore -> connection refused, fast

	// A fleet-managed relay is never a blocking prerequisite: an unreachable
	// relay reports "ok" (fleet downloads/starts it on launch), not "error".
	c := checkRelay(url, "download")
	if c.Status != "ok" {
		t.Fatalf("expected status ok for an unreachable (fleet-managed) relay, got %q", c.Status)
	}
}

func TestCheckRelayReportsManagedWhenBinaryPresent(t *testing.T) {
	// Unreachable but the managed binary exists → "managed by fleet", not a failure.
	c := relayCheckFor(false /*reachable*/, true /*binaryPresent*/, "download")
	if c.Status != "ok" || !strings.Contains(c.Detail, "managed") {
		t.Errorf("expected managed-by-fleet ok, got %+v", c)
	}
}

func TestCheckRelayEmbeddedBackend(t *testing.T) {
	// Embedded backend, unreachable: honest "embedded coord" message, never the
	// AGPL-download wording.
	c := relayCheckFor(false /*reachable*/, false /*binaryPresent*/, "embedded")
	if c.Status != "ok" || !strings.Contains(c.Detail, "embedded coord") {
		t.Errorf("expected embedded-coord ok, got %+v", c)
	}
	if strings.Contains(c.Detail, "download") {
		t.Errorf("embedded backend must not mention download: %q", c.Detail)
	}
}

// The doctor must not assume Homebrew exists everywhere: brew is the hint on
// darwin only, apt on linux, and a generic hint elsewhere.
func TestInstallHintPerOS(t *testing.T) {
	cases := []struct{ goos, pkg, want string }{
		{"darwin", "tmux", "brew install tmux"},
		{"linux", "tmux", "sudo apt install tmux"},
		{"freebsd", "tmux", "install tmux with your system package manager"},
	}
	for _, tc := range cases {
		if got := installHint(tc.goos, tc.pkg); got != tc.want {
			t.Errorf("installHint(%q, %q) = %q, want %q", tc.goos, tc.pkg, got, tc.want)
		}
	}
}

// The tmux check must route its FixCmd through installHint: a hardcoded
// "brew install tmux" would be wrong everywhere but darwin. Pinned per-OS so
// reverting the check to a brew-only hint fails here.
func TestTmuxCheckMissingRoutesThroughInstallHint(t *testing.T) {
	probeErr := errors.New("exec: \"tmux\": executable file not found in $PATH")
	for _, goos := range []string{"darwin", "linux", "freebsd"} {
		c := tmuxCheck(goos, "", probeErr)
		if c.Status != "missing" {
			t.Fatalf("goos %s: expected status missing, got %q", goos, c.Status)
		}
		if want := installHint(goos, "tmux"); c.FixCmd != want {
			t.Errorf("goos %s: FixCmd = %q, want %q", goos, c.FixCmd, want)
		}
	}
}

func TestTmuxCheckOK(t *testing.T) {
	c := tmuxCheck("linux", "tmux 3.4", nil)
	if c.Status != "ok" || c.Detail != "tmux 3.4" || c.FixCmd != "" {
		t.Fatalf("expected ok check with version detail and no FixCmd, got %+v", c)
	}
}

// The iTerm2 check is macOS-only: it must be skipped entirely off-darwin
// instead of reporting a bogus "missing" with a brew cask hint.
func TestRunSkipsITerm2OffDarwin(t *testing.T) {
	srv := healthyRelay()
	defer srv.Close()

	for _, c := range run(srv.URL, "download", "linux") {
		if c.Name == "iTerm2" {
			t.Fatal("iTerm2 check must be skipped on non-darwin platforms")
		}
	}

	found := false
	for _, c := range run(srv.URL, "download", "darwin") {
		if c.Name == "iTerm2" {
			found = true
		}
	}
	if !found {
		t.Fatal("iTerm2 check must be present on darwin")
	}
}

// The tmux probe must run exactly `tmux -V` and feed its output into the
// pure tmuxCheck builder.
func TestCheckTmuxArgv(t *testing.T) {
	calls := swapExecCommand(t, func() *exec.Cmd { return exec.Command("echo", "tmux 3.4") })

	c := checkTmux("linux")
	if want := [][]string{{"tmux", "-V"}}; !reflect.DeepEqual(*calls, want) {
		t.Fatalf("checkTmux argv = %v, want %v", *calls, want)
	}
	if c.Status != "ok" || c.Detail != "tmux 3.4" {
		t.Fatalf("probe output must flow into the check, got %+v", c)
	}
}

// A failing tmux probe must flow through tmuxCheck into a missing check with
// the per-OS install hint.
func TestCheckTmuxProbeErrorFlowsToBuilder(t *testing.T) {
	swapExecCommand(t, failingCmd)

	c := checkTmux("linux")
	if c.Status != "missing" {
		t.Fatalf("expected status missing for a failed probe, got %q", c.Status)
	}
	if want := installHint("linux", "tmux"); c.FixCmd != want {
		t.Errorf("FixCmd = %q, want %q", c.FixCmd, want)
	}
}

// The claude probe must run exactly `claude --version` and surface its output.
func TestCheckClaudeArgv(t *testing.T) {
	calls := swapExecCommand(t, func() *exec.Cmd { return exec.Command("echo", "1.2.3 (Claude Code)") })

	c := checkClaude()
	if want := [][]string{{"claude", "--version"}}; !reflect.DeepEqual(*calls, want) {
		t.Fatalf("checkClaude argv = %v, want %v", *calls, want)
	}
	if c.Status != "ok" || c.Detail != "1.2.3 (Claude Code)" {
		t.Fatalf("probe output must flow into the check, got %+v", c)
	}
}

// A failing claude probe must produce a missing check with the npm install hint.
func TestCheckClaudeProbeErrorFlowsToBuilder(t *testing.T) {
	swapExecCommand(t, failingCmd)

	c := checkClaude()
	if c.Status != "missing" {
		t.Fatalf("expected status missing for a failed probe, got %q", c.Status)
	}
	if want := "npm install -g @anthropic-ai/claude-code"; c.FixCmd != want {
		t.Errorf("FixCmd = %q, want %q", c.FixCmd, want)
	}
}

// TestCheckRelayDoesNotHang is the regression test for the original bug:
// the old doctor used an unbounded `curl GET /mcp`, which blocks forever on
// the relay's SSE stream. checkRelay must return promptly even when the relay
// accepts the connection but never sends a usable response.
func TestCheckRelayDoesNotHang(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // hold the connection open, like an SSE stream
	}))
	defer func() {
		close(block)
		srv.Close()
	}()

	done := make(chan Check, 1)
	go func() { done <- checkRelay(srv.URL, "download") }()

	select {
	case c := <-done:
		// The point of this test is the bounded probe (no hang); a managed
		// relay that doesn't answer still reports "ok", not a failure.
		if c.Status != "ok" {
			t.Fatalf("expected ok status for an unresponsive (fleet-managed) relay, got %q", c.Status)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("checkRelay hung on a non-responding relay (no timeout)")
	}
}
