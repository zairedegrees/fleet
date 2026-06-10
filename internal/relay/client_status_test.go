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

// The relay truncates the tasks array to `limit` (default 50) and computes
// `count` AFTER truncation — count is a page length, not a board total. The
// only way to keep busy boards honest is to request an explicit high limit.
func TestCountActiveTasksSendsHighLimit(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		tasksJSON := `{"tasks":[{"id":"t-1"},{"id":"t-2"},{"id":"t-3"}],"count":3}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":%s}]}}`, jsonEscape(tasksJSON))
	}))
	defer server.Close()

	n, err := NewClient(server.URL).CountActiveTasks("proj", "dev")
	if err != nil {
		t.Fatalf("CountActiveTasks failed: %v", err)
	}
	if n != 3 {
		t.Errorf("expected count == page length (3), got %d", n)
	}

	params, ok := gotBody["params"].(map[string]interface{})
	if !ok {
		t.Fatal("expected params object")
	}
	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		t.Fatal("expected arguments object")
	}
	limit, ok := args["limit"].(float64)
	if !ok {
		t.Fatal("expected an explicit limit in the request — without it the relay defaults to 50 and counts silently cap")
	}
	if limit < 500 {
		t.Errorf("limit must be high enough not to cap real workloads, got %v", limit)
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
