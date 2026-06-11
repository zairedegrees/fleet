package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return m
}

func TestProvisionMCPCreatesFreshFile(t *testing.T) {
	dir := t.TempDir()
	if err := ProvisionMCP(dir, "http://localhost:8090/mcp"); err != nil {
		t.Fatalf("ProvisionMCP: %v", err)
	}
	m := readJSON(t, filepath.Join(dir, ".mcp.json"))
	srv := m["mcpServers"].(map[string]any)["agent-relay"].(map[string]any)
	if srv["url"] != "http://localhost:8090/mcp" || srv["type"] != "http" {
		t.Errorf("bad agent-relay entry: %v", srv)
	}
}

func TestProvisionMCPMergesPreservingOtherServers(t *testing.T) {
	dir := t.TempDir()
	existing := `{"mcpServers":{"other":{"type":"stdio","command":"foo"}}}`
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(existing), 0644)

	if err := ProvisionMCP(dir, "http://localhost:8090/mcp"); err != nil {
		t.Fatalf("ProvisionMCP: %v", err)
	}
	m := readJSON(t, filepath.Join(dir, ".mcp.json"))
	servers := m["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("merge dropped the user's 'other' server")
	}
	if _, ok := servers["agent-relay"]; !ok {
		t.Error("merge did not add agent-relay")
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json.bak")); err != nil {
		t.Error("expected a .mcp.json.bak backup")
	}
}

func TestProvisionMCPMalformedExistingIsBackedUpNotClobbered(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte("{not json"), 0644)
	err := ProvisionMCP(dir, "http://localhost:8090/mcp")
	if err == nil {
		t.Fatal("expected an error on malformed existing .mcp.json")
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if string(b) != "{not json" {
		t.Error("malformed file must not be overwritten")
	}
}
