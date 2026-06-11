package dashboard

import (
	"os"
	"path/filepath"
	"testing"
)

// setup makes an isolated HOME and a project cwd, returns (home, cwd).
func setup(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	os.MkdirAll(filepath.Join(cwd, ".claude"), 0755)
	return home, cwd
}

func TestConfigureWritesWhenNoStatusLineAnywhere(t *testing.T) {
	home, cwd := setup(t)
	applied, err := Configure(cwd, home)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !applied {
		t.Fatal("expected Configure to apply when no status line exists")
	}
	local := filepath.Join(cwd, ".claude", "settings.local.json")
	if !hasStatusLine(local) {
		t.Error("status line should be written to settings.local.json")
	}
}

func TestConfigureSkipsWhenGlobalStatusLine(t *testing.T) {
	home, cwd := setup(t)
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"),
		[]byte(`{"statusLine":{"type":"command","command":"mine"}}`), 0644)
	applied, err := Configure(cwd, home)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if applied {
		t.Error("must not apply when the user has a global status line")
	}
	if _, err := os.Stat(filepath.Join(cwd, ".claude", "settings.local.json")); !os.IsNotExist(err) {
		t.Error("must not write settings.local.json when respecting the user")
	}
}

func TestConfigureSkipsWhenProjectStatusLine(t *testing.T) {
	home, cwd := setup(t)
	os.WriteFile(filepath.Join(cwd, ".claude", "settings.json"),
		[]byte(`{"statusLine":{"type":"command","command":"proj"}}`), 0644)
	applied, _ := Configure(cwd, home)
	if applied {
		t.Error("must not apply when a project status line exists")
	}
}
