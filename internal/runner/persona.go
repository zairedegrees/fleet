package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zairedegrees/fleet/internal/config"
)

// personaFilePath returns the on-disk location of an agent's persona prompt,
// namespaced by project under ~/.fleet/personas so it never collides with the
// user's hand-authored ~/.claude config. The name is fleet-generated from
// validName-constrained project/agent names, so the path carries no metachars
// and is safe to single-quote onto the launch line.
func personaFilePath(project, agent string) string {
	return filepath.Join(config.FleetDir(), "personas", fmt.Sprintf("%s-%s.txt", project, agent))
}

// WritePersonaFile writes an agent's persona to its file and returns the path,
// for passing to --append-system-prompt-file. An agent with no persona writes
// nothing and returns "" (the launch then omits the flag). The persona is
// written verbatim as a file body — no escaping, since a file is not re-parsed
// by any shell.
func WritePersonaFile(project string, a config.AgentConfig) (string, error) {
	if !a.HasPersona() {
		return "", nil
	}
	path := personaFilePath(project, a.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create personas dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(a.Persona), 0644); err != nil {
		return "", fmt.Errorf("write persona for %s: %w", a.Name, err)
	}
	return path, nil
}
