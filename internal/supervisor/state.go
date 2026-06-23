package supervisor

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/zairedegrees/fleet/internal/config"
)

func StatePath(project string) string {
	return filepath.Join(config.FleetDir(), project+".supervisor.json")
}

// LoadState reads the project's supervisor state. A missing file is not an
// error: it returns a fresh, empty state.
func LoadState(project string) (*State, error) {
	path := StatePath(project)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &State{Project: project, Agents: map[string]*AgentState{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.Agents == nil {
		st.Agents = map[string]*AgentState{}
	}
	st.Project = project
	return &st, nil
}

// SaveState writes state atomically (temp + rename) so a crash never leaves a
// half-written file.
func SaveState(st *State) error {
	dir := config.FleetDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := StatePath(st.Project) + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, StatePath(st.Project))
}

func ClearState(project string) error {
	err := os.Remove(StatePath(project))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
