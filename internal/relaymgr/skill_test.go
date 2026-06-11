package relaymgr

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRelaySkillInstallsWhenMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("---\nname: relay\n---\nbody"))
	}))
	defer srv.Close()
	skillRawURL = srv.URL
	defer func() { skillRawURL = defaultSkillRawURL }()

	skillsDir := t.TempDir()
	dest := filepath.Join(skillsDir, "relay", "SKILL.md")
	if err := ensureRelaySkillAt(dest); err != nil {
		t.Fatalf("ensureRelaySkillAt: %v", err)
	}
	b, err := os.ReadFile(dest)
	if err != nil || len(b) == 0 {
		t.Fatalf("skill not installed: %v", err)
	}
	if err := ensureRelaySkillAt(dest); err != nil {
		t.Errorf("second call should be a no-op, got %v", err)
	}
}
