package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListAgents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentsJSON := `{"agents":[{"name":"ops","role":"monitor","status":"active"},{"name":"quant","role":"analyst","status":"inactive"}],"count":2}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":%s}]}}`, jsonEscape(agentsJSON))
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
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":%s}]}}`,
			jsonEscape(`{"agents":[{"name":"dev","role":"Developer","status":"active","profile_slug":"dev","reports_to":"","is_executive":false,"color":"blue"}]}`))
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

func TestPushVaultDoc(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		resp := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"ok\":true}"}]}}`
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.PushVaultDoc("my-project", "shared/arch.md", []byte("# Architecture"))
	if err != nil {
		t.Fatalf("PushVaultDoc failed: %v", err)
	}

	params, ok := gotBody["params"].(map[string]interface{})
	if !ok {
		t.Fatal("expected params object")
	}
	if params["name"] != "set_memory" {
		t.Errorf("expected tool name 'set_memory', got %v", params["name"])
	}
	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		t.Fatal("expected arguments object")
	}
	if args["key"] != "vault:shared/arch.md" {
		t.Errorf("expected key 'vault:shared/arch.md', got %v", args["key"])
	}
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
