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

	// The advertised catalog and the dispatch registry must agree exactly — an
	// advertised-but-unimplemented tool (or vice versa) would silently break a
	// real agent's discovery.
	for name := range handlers {
		if !listed[name] {
			t.Errorf("handler %q is not advertised in tools/list", name)
		}
	}
	for name := range listed {
		if _, ok := handlers[name]; !ok {
			t.Errorf("advertised tool %q has no handler", name)
		}
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
