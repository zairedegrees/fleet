package coord

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolsListMatchesHandlers(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	res, b := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, b)
	}
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}
	if len(resp.Result.Tools) == 0 {
		t.Fatal("tools/list returned no tools")
	}

	listed := map[string]bool{}
	for _, td := range resp.Result.Tools {
		if td.Name == "" || td.Description == "" {
			t.Errorf("tool missing name/description: %+v", td)
		}
		if td.InputSchema["type"] != "object" {
			t.Errorf("%s inputSchema is not an object", td.Name)
		}
		listed[td.Name] = true
	}

	// Advertised tools must all have handlers; handlers may be advertised OR
	// operator-only (handled on tools/call by the fleet CLI but kept out of the
	// agents' catalog to save context tokens).
	for name := range handlers {
		if !listed[name] && !operatorOnly[name] {
			t.Errorf("handler %q is neither advertised nor operator-only", name)
		}
	}
	for name := range listed {
		if _, ok := handlers[name]; !ok {
			t.Errorf("advertised tool %q has no handler", name)
		}
		if operatorOnly[name] {
			t.Errorf("operator-only tool %q must not be advertised", name)
		}
	}
}

func TestOperatorOnlyToolsStillDispatch(t *testing.T) {
	s := New(newTestStore(t))
	// register_agent is not advertised, but the fleet CLI calls it by name —
	// it must still execute, not return "not supported".
	res := mustCall(t, s, "register_agent", map[string]any{"name": "dev", "project": "p"})
	var got struct {
		Agent struct {
			Name string `json:"name"`
		} `json:"agent"`
	}
	decodePayload(t, res, &got)
	if got.Agent.Name != "dev" {
		t.Errorf("register_agent must still work via tools/call, got %+v", got)
	}
}

func TestInitializeAdvertisesToolsCapability(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	_, b := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	var resp struct {
		Result struct {
			ProtocolVersion string         `json:"protocolVersion"`
			Capabilities    map[string]any `json:"capabilities"`
			ServerInfo      map[string]any `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Result.ProtocolVersion == "" {
		t.Error("initialize missing protocolVersion")
	}
	if _, ok := resp.Result.Capabilities["tools"]; !ok {
		t.Errorf("initialize did not advertise the tools capability: %+v", resp.Result.Capabilities)
	}
	if resp.Result.ServerInfo["name"] == nil {
		t.Error("initialize missing serverInfo.name")
	}
}

func TestGetMCPIsMethodNotAllowed(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	// coord pushes no server-initiated messages, so the optional SSE GET stream is
	// declined with 405 — spec-compliant, clients fall back to POST-only.
	if rec.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp = %d, want 405", rec.Result().StatusCode)
	}
}

func TestPingAndInitializedNotification(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	if resp, _ := rawCall(t, h, `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`); resp.Error != nil {
		t.Errorf("ping returned error: %v", resp.Error)
	}
	res, _ := postMCP(t, h, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if res.StatusCode != http.StatusAccepted {
		t.Errorf("notifications/initialized = %d, want 202", res.StatusCode)
	}
}
