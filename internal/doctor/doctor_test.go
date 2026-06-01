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
