package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method != "tools/call" {
			t.Errorf("unexpected method: %s", req.Method)
		}

		resp := mpcResponse{
			Result: json.RawMessage(`{"content":[{"type":"text","text":"{\"projects\":[\"proj-a\",\"proj-b\"]}"}]}`),
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

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
