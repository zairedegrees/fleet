package dashboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// hasStatusLine reports whether the JSON settings file at path defines a
// non-empty statusLine. Absent file or malformed JSON → false.
func hasStatusLine(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var s struct {
		StatusLine map[string]any `json:"statusLine"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return false
	}
	return len(s.StatusLine) > 0
}

// mergeStatusLine sets statusLine to a command entry in the JSON settings file,
// preserving every other key. A fresh file is created when absent; an existing
// file is backed up (.bak) then merged; a malformed existing file is refused.
func mergeStatusLine(path, command string) error {
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

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	root["statusLine"] = map[string]any{"type": "command", "command": command}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}
