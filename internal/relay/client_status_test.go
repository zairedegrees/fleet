package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fleet --status counts an agent's non-done tasks via list_tasks filtered by
// profile slug + status=active — the relay is the source of truth, not the
// tmux pane content.
func TestCountActiveTasks(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		tasksJSON := `{"tasks":[{"id":"t-1","status":"pending"},{"id":"t-2","status":"in-progress"}]}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":%s}]}}`, jsonEscape(tasksJSON))
	}))
	defer server.Close()

	n, err := NewClient(server.URL).CountActiveTasks("proj", "dev")
	if err != nil {
		t.Fatalf("CountActiveTasks failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 active tasks, got %d", n)
	}

	params, ok := gotBody["params"].(map[string]interface{})
	if !ok {
		t.Fatal("expected params object")
	}
	if params["name"] != "list_tasks" {
		t.Errorf("expected tool name 'list_tasks', got %v", params["name"])
	}
	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		t.Fatal("expected arguments object")
	}
	if args["project"] != "proj" || args["profile"] != "dev" {
		t.Errorf("expected project+profile filters, got %v", args)
	}
	if args["status"] != "active" {
		t.Errorf("expected status 'active', got %v", args["status"])
	}
}

// The relay truncates the tasks array at `limit` — when a count field is
// present it is authoritative over len(tasks).
func TestCountActiveTasksPrefersCountField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tasksJSON := `{"tasks":[{"id":"t-1","status":"pending"}],"count":7}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":%s}]}}`, jsonEscape(tasksJSON))
	}))
	defer server.Close()

	n, err := NewClient(server.URL).CountActiveTasks("proj", "dev")
	if err != nil {
		t.Fatalf("CountActiveTasks failed: %v", err)
	}
	if n != 7 {
		t.Errorf("expected count field (7) to win over len(tasks), got %d", n)
	}
}

// A relay failure must surface as an error, never as a fake 0.
func TestCountActiveTasksSurfacesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"no such project"}],"isError":true}}`)
	}))
	defer server.Close()

	if _, err := NewClient(server.URL).CountActiveTasks("proj", "dev"); err == nil {
		t.Fatal("expected relay tool error to surface, got nil")
	}
}
