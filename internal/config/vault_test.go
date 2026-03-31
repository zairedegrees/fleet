package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setupVaultFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, ".fleet", "vault")

	// shared/
	os.MkdirAll(filepath.Join(vaultDir, "shared"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "shared", "arch.md"), []byte("# Architecture"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "shared", "conventions.md"), []byte("# Conventions"), 0644)

	// dev/
	os.MkdirAll(filepath.Join(vaultDir, "dev"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "dev", "api.md"), []byte("# API Guide"), 0644)

	// auditor/
	os.MkdirAll(filepath.Join(vaultDir, "auditor"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "auditor", "tests.md"), []byte("# Test Strategy"), 0644)

	// ops/
	os.MkdirAll(filepath.Join(vaultDir, "ops"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "ops", "deploy.md"), []byte("# Deploy Runbook"), 0644)

	// non-md file (should be ignored)
	os.WriteFile(filepath.Join(vaultDir, "shared", "notes.txt"), []byte("ignore me"), 0644)

	return vaultDir
}

func TestResolveVaultDocs_DevAgent(t *testing.T) {
	vaultDir := setupVaultFixture(t)

	agent := AgentConfig{Name: "dev", Role: "Lead developer", Color: "green"}
	docs, err := agent_resolve(vaultDir, agent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get: shared/arch.md, shared/conventions.md, dev/api.md
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs for dev agent, got %d: %v", len(docs), docPaths(docs))
	}
}

func TestResolveVaultDocs_AuditorAgent(t *testing.T) {
	vaultDir := setupVaultFixture(t)

	agent := AgentConfig{Name: "auditor", Role: "Code review & auditing", Color: "orange"}
	docs, err := agent_resolve(vaultDir, agent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get: shared/* (2) + auditor/* (1) = 3
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs for auditor agent, got %d: %v", len(docs), docPaths(docs))
	}
}

func TestResolveVaultDocs_RoleMatch(t *testing.T) {
	vaultDir := setupVaultFixture(t)

	// Agent named "reviewer" but role contains "audit" — should match auditor/ dir
	agent := AgentConfig{Name: "reviewer", Role: "Security auditor", Color: "red"}
	docs, err := agent_resolve(vaultDir, agent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get: shared/* (2) + auditor/* (1, role match) = 3
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs for reviewer agent, got %d: %v", len(docs), docPaths(docs))
	}
}

func TestResolveVaultDocs_OpsAgent(t *testing.T) {
	vaultDir := setupVaultFixture(t)

	agent := AgentConfig{Name: "ops", Role: "Infrastructure", Color: "purple"}
	docs, err := agent_resolve(vaultDir, agent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get: shared/* (2) + ops/* (1) = 3
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs for ops agent, got %d: %v", len(docs), docPaths(docs))
	}
}

func TestResolveVaultDocs_NoVaultDir(t *testing.T) {
	agent := AgentConfig{Name: "dev", Role: "Dev", Color: "green"}
	docs, err := agent_resolve("/nonexistent/path", agent)
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if docs != nil {
		t.Fatalf("expected nil docs for missing dir, got %d", len(docs))
	}
}

func TestVaultSize(t *testing.T) {
	docs := []VaultDoc{
		{Path: "a.md", Content: []byte("hello")},    // 5 bytes
		{Path: "b.md", Content: []byte("world!!!")}, // 8 bytes
	}
	if VaultSize(docs) != 13 {
		t.Errorf("expected 13, got %d", VaultSize(docs))
	}
}

// helpers
func agent_resolve(vaultDir string, agent AgentConfig) ([]VaultDoc, error) {
	return ResolveVaultDocs(vaultDir, agent)
}

func docPaths(docs []VaultDoc) []string {
	var paths []string
	for _, d := range docs {
		paths = append(paths, d.Path)
	}
	return paths
}
