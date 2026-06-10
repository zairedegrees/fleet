package relay

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fleet stop must deregister the agent from the relay so it stops showing up as
// a ghost in list_agents / routing. DeactivateAgent issues the deactivate_agent
// tool call with the agent name + project.
func TestDeactivateAgent(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"ok\":true}"}]}}`)
	}))
	defer server.Close()

	if err := NewClient(server.URL).DeactivateAgent("dev", "proj"); err != nil {
		t.Fatalf("DeactivateAgent failed: %v", err)
	}
	if !strings.Contains(gotBody, "deactivate_agent") {
		t.Errorf("expected a deactivate_agent call, got: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"dev"`) || !strings.Contains(gotBody, `"proj"`) {
		t.Errorf("expected name+project in the args, got: %s", gotBody)
	}
}

// An added agent can only receive dispatched tasks if it is registered on the
// relay with a profile_slug — RegisterAgent issues that register_agent call.
func TestRegisterAgent(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"ok\":true}"}]}}`)
	}))
	defer server.Close()

	if err := NewClient(server.URL).RegisterAgent("dev", "proj", "Developer", "dev"); err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}
	if !strings.Contains(gotBody, "register_agent") {
		t.Errorf("expected a register_agent call, got: %s", gotBody)
	}
	if !strings.Contains(gotBody, "profile_slug") || !strings.Contains(gotBody, `"dev"`) {
		t.Errorf("expected profile_slug + name in the args, got: %s", gotBody)
	}
}
