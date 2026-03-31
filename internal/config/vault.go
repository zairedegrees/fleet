package config

import (
	"os"
	"path/filepath"
	"strings"
)

// VaultDoc represents a vault document to inject into an agent.
type VaultDoc struct {
	Path    string // relative path within vault dir (e.g. "shared/architecture.md")
	Content []byte
}

// ResolveVaultDocs returns the vault docs that should be injected for the given agent.
// It matches: shared/* + {agent.Name}/* + {agent.Role}/* (case-insensitive dir match).
// Returns nil if vault dir doesn't exist.
func ResolveVaultDocs(vaultDir string, agent AgentConfig) ([]VaultDoc, error) {
	if _, err := os.Stat(vaultDir); os.IsNotExist(err) {
		return nil, nil
	}

	var docs []VaultDoc

	entries, err := os.ReadDir(vaultDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()

		if !shouldIncludeDir(dirName, agent) {
			continue
		}

		subDir := filepath.Join(vaultDir, dirName)
		files, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			fullPath := filepath.Join(subDir, f.Name())
			content, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			docs = append(docs, VaultDoc{
				Path:    filepath.Join(dirName, f.Name()),
				Content: content,
			})
		}
	}

	return docs, nil
}

// shouldIncludeDir returns true if the vault subdirectory should be included for this agent.
// Matches: "shared" (always), agent name (exact, case-insensitive), or agent role (contains, case-insensitive).
func shouldIncludeDir(dirName string, agent AgentConfig) bool {
	lower := strings.ToLower(dirName)

	if lower == "shared" {
		return true
	}
	if strings.EqualFold(dirName, agent.Name) {
		return true
	}
	if agent.Role != "" && strings.Contains(strings.ToLower(agent.Role), lower) {
		return true
	}
	return false
}

// VaultSize returns total bytes of vault docs.
func VaultSize(docs []VaultDoc) int64 {
	var total int64
	for _, d := range docs {
		total += int64(len(d.Content))
	}
	return total
}

const VaultSizeWarningBytes = 50 * 1024 // 50KB
