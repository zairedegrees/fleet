package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureWritesGitignore(t *testing.T) {
	home, cwd := setup(t)
	applied, err := Configure(cwd, home)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !applied {
		t.Fatal("expected Configure to apply")
	}
	b, err := os.ReadFile(filepath.Join(cwd, ".claude", ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	got := string(b)
	for _, want := range []string{"settings.local.json", "settings.local.json.bak"} {
		found := false
		for _, l := range strings.Split(got, "\n") {
			if strings.TrimSpace(l) == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf(".gitignore should contain %q, got:\n%s", want, got)
		}
	}
}

func TestEnsureGitignorePreservesAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("foo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignore(dir); err != nil {
		t.Fatalf("ensureGitignore: %v", err)
	}
	b, _ := os.ReadFile(path)
	got := string(b)
	for _, want := range []string{"foo", "settings.local.json", "settings.local.json.bak"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q preserved/added, got:\n%s", want, got)
		}
	}

	// Idempotent: a second call adds nothing.
	if err := ensureGitignore(dir); err != nil {
		t.Fatalf("ensureGitignore (2nd): %v", err)
	}
	b2, _ := os.ReadFile(path)
	if string(b2) != got {
		t.Errorf("second call mutated file:\nfirst:\n%s\nsecond:\n%s", got, string(b2))
	}
}

func TestConfigureSkipDoesNotCreateGitignore(t *testing.T) {
	home, cwd := setup(t)
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"),
		[]byte(`{"statusLine":{"type":"command","command":"mine"}}`), 0644)
	applied, err := Configure(cwd, home)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if applied {
		t.Fatal("expected Configure to skip")
	}
	if _, err := os.Stat(filepath.Join(cwd, ".claude", ".gitignore")); !os.IsNotExist(err) {
		t.Error("must not create .gitignore when skipping")
	}
}
