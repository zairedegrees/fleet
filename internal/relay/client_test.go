package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentsJSON := `{"agents":[{"name":"a1","project":"proj-a"},{"name":"a2","project":"proj-b"}]}`
		resp := mpcResponse{
			Result: json.RawMessage(`{"content":[{"type":"text","text":` + jsonEscape(agentsJSON) + `}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	projects, err := client.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0] != "proj-a" {
		t.Errorf("expected proj-a, got %s", projects[0])
	}
}

func TestListAgents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentsJSON := `{"agents":[{"name":"ops","role":"monitor","status":"active"},{"name":"quant","role":"analyst","status":"inactive"}],"count":2}`
		resp := mpcResponse{
			Result: json.RawMessage(`{"content":[{"type":"text","text":` + jsonEscape(agentsJSON) + `}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	agents, err := client.ListAgents("test-project")
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[0].Name != "ops" {
		t.Errorf("expected ops, got %s", agents[0].Name)
	}
}

func TestClientTimeout(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
		}
	}))
	defer func() {
		close(done)
		srv.Close()
	}()

	client := NewClient(srv.URL)
	err := client.Health()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "unreachable") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected timeout-related error, got: %v", err)
	}
}

func TestListAgentsParseColor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"agents\":[{\"name\":\"dev\",\"role\":\"Developer\",\"status\":\"active\",\"profile_slug\":\"dev\",\"reports_to\":\"\",\"is_executive\":false,\"color\":\"blue\"}]}"}]}}`
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	agents, err := client.ListAgents("test")
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Color != "blue" {
		t.Errorf("expected color 'blue', got %q", agents[0].Color)
	}
}

func TestDispatchTask(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		resp := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"task_id\":\"t-123\"}"}]}}`
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.DispatchTask("dev", "my-project", "Fix the bug")
	if err != nil {
		t.Fatalf("DispatchTask failed: %v", err)
	}

	params, ok := gotBody["params"].(map[string]interface{})
	if !ok {
		t.Fatal("expected params object")
	}
	if params["name"] != "dispatch_task" {
		t.Errorf("expected tool name 'dispatch_task', got %v", params["name"])
	}
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
