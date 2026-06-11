package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func allowList(t *testing.T, m map[string]any) []any {
	t.Helper()
	perms, ok := m["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("no permissions object: %v", m)
	}
	allow, ok := perms["allow"].([]any)
	if !ok {
		t.Fatalf("no permissions.allow array: %v", perms)
	}
	return allow
}

func hasRule(allow []any, rule string) bool {
	for _, e := range allow {
		if s, ok := e.(string); ok && s == rule {
			return true
		}
	}
	return false
}

func TestProvisionRelayPermissionsCreatesFreshFile(t *testing.T) {
	dir := t.TempDir()
	if err := ProvisionRelayPermissions(dir); err != nil {
		t.Fatalf("ProvisionRelayPermissions: %v", err)
	}
	m := readJSON(t, filepath.Join(dir, ".claude", "settings.local.json"))
	if !hasRule(allowList(t, m), "mcp__agent-relay__*") {
		t.Errorf("fresh file missing relay allow rule: %v", m)
	}
}

func TestProvisionRelayPermissionsPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	existing := `{"statusLine":{"type":"command","command":"foo"},"permissions":{"allow":["Bash(ls)"]}}`
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(existing), 0644)

	if err := ProvisionRelayPermissions(dir); err != nil {
		t.Fatalf("ProvisionRelayPermissions: %v", err)
	}
	m := readJSON(t, filepath.Join(claudeDir, "settings.local.json"))
	if _, ok := m["statusLine"]; !ok {
		t.Error("merge dropped the existing statusLine key")
	}
	allow := allowList(t, m)
	if !hasRule(allow, "Bash(ls)") {
		t.Error("merge dropped the existing Bash(ls) allow entry")
	}
	if !hasRule(allow, "mcp__agent-relay__*") {
		t.Error("merge did not add the relay allow rule")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "settings.local.json.bak")); err != nil {
		t.Error("expected a settings.local.json.bak backup")
	}
}

func TestProvisionRelayPermissionsIdempotent(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := ProvisionRelayPermissions(dir); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	m := readJSON(t, filepath.Join(dir, ".claude", "settings.local.json"))
	count := 0
	for _, e := range allowList(t, m) {
		if s, ok := e.(string); ok && s == "mcp__agent-relay__*" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("rule must appear exactly once after two runs, got %d", count)
	}
}

func TestProvisionRelayPermissionsMalformedExistingIsNotClobbered(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	path := filepath.Join(claudeDir, "settings.local.json")
	os.WriteFile(path, []byte("{not json"), 0644)

	if err := ProvisionRelayPermissions(dir); err == nil {
		t.Fatal("expected an error on malformed existing settings.local.json")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{not json" {
		t.Error("malformed file must not be overwritten")
	}
}
