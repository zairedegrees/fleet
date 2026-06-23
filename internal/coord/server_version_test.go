package coord

import (
	"encoding/json"
	"testing"

	"github.com/zairedegrees/fleet/internal/version"
)

// The coord's MCP serverInfo.version must come from the shared version package,
// not a hardcoded literal — one source of truth with the CLI.
func TestInitializeServerInfoVersionMatchesBuild(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	_, b := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	var resp struct {
		Result struct {
			ServerInfo map[string]any `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if got := resp.Result.ServerInfo["version"]; got != version.Version {
		t.Fatalf("serverInfo.version = %v, want %q (single source of truth)", got, version.Version)
	}
}
