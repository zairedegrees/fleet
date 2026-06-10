package doctor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

	c := checkRelay(srv.URL)
	if c.Status != "ok" {
		t.Fatalf("expected status ok, got %q (detail: %s)", c.Status, c.Detail)
	}
}

func TestCheckRelayUnreachable(t *testing.T) {
	srv := healthyRelay()
	url := srv.URL
	srv.Close() // nothing listening anymore -> connection refused, fast

	c := checkRelay(url)
	if c.Status != "error" {
		t.Fatalf("expected status error for a down relay, got %q", c.Status)
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

// The iTerm2 check is macOS-only: it must be skipped entirely off-darwin
// instead of reporting a bogus "missing" with a brew cask hint.
func TestRunSkipsITerm2OffDarwin(t *testing.T) {
	srv := healthyRelay()
	defer srv.Close()

	for _, c := range run(srv.URL, "linux") {
		if c.Name == "iTerm2" {
			t.Fatal("iTerm2 check must be skipped on non-darwin platforms")
		}
	}

	found := false
	for _, c := range run(srv.URL, "darwin") {
		if c.Name == "iTerm2" {
			found = true
		}
	}
	if !found {
		t.Fatal("iTerm2 check must be present on darwin")
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
	go func() { done <- checkRelay(srv.URL) }()

	select {
	case c := <-done:
		if c.Status != "error" {
			t.Fatalf("expected error status for an unresponsive relay, got %q", c.Status)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("checkRelay hung on a non-responding relay (no timeout)")
	}
}
