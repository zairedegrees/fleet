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

// The relay's re-register is a full-replace UPDATE: any field fleet omits is
// reset server-side. The full registration must therefore carry reports_to and
// is_executive on the wire alongside profile_slug.
func TestRegisterAgentFullSendsHierarchyFields(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"ok\":true}"}]}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.RegisterAgentFull(AgentRegistration{
		Name: "lead", Project: "proj", Role: "Tech Lead",
		ProfileSlug: "lead", ReportsTo: "ceo", IsExecutive: true,
	})
	if err != nil {
		t.Fatalf("RegisterAgentFull failed: %v", err)
	}

	params, ok := gotBody["params"].(map[string]interface{})
	if !ok {
		t.Fatal("expected params object")
	}
	if params["name"] != "register_agent" {
		t.Errorf("expected tool name 'register_agent', got %v", params["name"])
	}
	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		t.Fatal("expected arguments object")
	}
	want := map[string]interface{}{
		"name": "lead", "project": "proj", "role": "Tech Lead",
		"profile_slug": "lead", "reports_to": "ceo", "is_executive": true,
	}
	for k, v := range want {
		if args[k] != v {
			t.Errorf("expected %s=%v on the wire, got %v", k, v, args[k])
		}
	}
}

// The 4-arg RegisterAgent (kept for existing callers) is a full registration
// with zero hierarchy: it must still send reports_to/is_executive explicitly
// so the full-replace re-register resets them to fleet's truth, not garbage.
func TestRegisterAgentSendsZeroHierarchy(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"ok\":true}"}]}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	if err := client.RegisterAgent("dev", "proj", "Dev", "dev"); err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	args := gotBody["params"].(map[string]interface{})["arguments"].(map[string]interface{})
	if args["profile_slug"] != "dev" {
		t.Errorf("expected profile_slug=dev, got %v", args["profile_slug"])
	}
	if args["is_executive"] != false {
		t.Errorf("expected is_executive=false on the wire, got %v", args["is_executive"])
	}
	if args["reports_to"] != "" {
		t.Errorf("expected reports_to=\"\" on the wire, got %v", args["reports_to"])
	}
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
