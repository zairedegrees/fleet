package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProvisionMCP writes/merges the agent-relay MCP server entry into the project's
// .mcp.json so spawned Claude Code agents reach the relay. A fresh file is
// created when absent; an existing file is merged non-destructively (every other
// server and field is preserved) after a .mcp.json.bak backup. A malformed
// existing file is refused (error) rather than clobbered.
func ProvisionMCP(projectCwd, relayURL string) error {
	path := filepath.Join(projectCwd, ".mcp.json")
	entry := map[string]any{"type": "http", "url": relayURL}

	root := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &root); err != nil {
			return fmt.Errorf("existing %s is not valid JSON (left untouched): %w", path, err)
		}
		if err := os.WriteFile(path+".bak", b, 0644); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["agent-relay"] = entry
	root["mcpServers"] = servers

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal .mcp.json: %w", err)
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}
