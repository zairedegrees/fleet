package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// relayPermissionRule is the Claude Code permission rule that pre-approves every
// agent-relay MCP tool, so a woken agent runs the relay lifecycle (claim/start/
// complete task, send_message, …) without an interactive prompt. The server
// segment must match the key ProvisionMCP writes into .mcp.json ("agent-relay").
const relayPermissionRule = "mcp__agent-relay__*"

// ProvisionRelayPermissions adds the relay allow-rule to the project's
// .claude/settings.local.json so launched agents reach the relay unattended
// without --dangerously-skip-permissions. It merges non-destructively: a fresh
// file is created when absent; an existing file is backed up (.bak) then merged,
// preserving every other key and every existing permissions.allow entry; the
// rule is added only if missing (idempotent). A malformed existing file is
// refused (error) rather than clobbered. settings.local.json is expected to be
// gitignored by fleet's dashboard setup, so nothing lands in the committed tree.
func ProvisionRelayPermissions(projectCwd string) error {
	path := filepath.Join(projectCwd, ".claude", "settings.local.json")

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

	perms, _ := root["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	allow, _ := perms["allow"].([]any)

	found := false
	for _, e := range allow {
		if s, ok := e.(string); ok && s == relayPermissionRule {
			found = true
			break
		}
	}
	if !found {
		allow = append(allow, relayPermissionRule)
	}
	perms["allow"] = allow
	root["permissions"] = perms

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.local.json: %w", err)
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}
