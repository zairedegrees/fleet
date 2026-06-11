package dashboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

func TestEnsureInstalledExtractsAsset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := EnsureInstalled()
	if err != nil {
		t.Fatalf("EnsureInstalled: %v", err)
	}
	if path != filepath.Join(config.FleetDir(), "dashboard", "index.mjs") {
		t.Errorf("unexpected install path %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		t.Fatalf("asset not written: %v", err)
	}
	if len(b) < 100000 {
		t.Errorf("embedded asset too small (%d bytes), expected the real ~127KB build", len(b))
	}
}

func TestEnsureInstalledIdempotentAndRefreshes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, _ := EnsureInstalled()

	os.WriteFile(path, []byte("STALE"), 0644)
	if _, err := EnsureInstalled(); err != nil {
		t.Fatalf("re-EnsureInstalled: %v", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) == "STALE" {
		t.Error("EnsureInstalled must refresh when bytes differ from the embedded asset")
	}
}
