// Package dashboard bundles the MIT-licensed claude-dashboard status line
// (uppinote20/claude-dashboard, vendored in dashboard.mjs) so fleet agents get
// rich per-pane observability without a separate plugin install.
package dashboard

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/zairedegrees/fleet/internal/config"
)

//go:embed dashboard.mjs
var asset []byte

// installPath is where the embedded bundle is extracted for agents to run.
func installPath() string {
	return filepath.Join(config.FleetDir(), "dashboard", "index.mjs")
}

// EnsureInstalled writes the embedded bundle to ~/.fleet/dashboard/index.mjs when
// it is absent or its bytes differ (so a fleet upgrade refreshes it), and returns
// the absolute path. The .mjs extension forces ESM mode so node runs it on 18+.
func EnsureInstalled() (string, error) {
	path := installPath()
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, asset) {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, asset, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// Configure gives the agents in cwd the bundled status line — but only as a
// fallback: if the user already defines a statusLine globally
// (~/.claude/settings.json), per-project (<cwd>/.claude/settings.json) or
// per-project-local (<cwd>/.claude/settings.local.json), it leaves everything
// untouched and returns applied=false. Otherwise it merges the bundled command
// into <cwd>/.claude/settings.local.json (gitignored, non-destructive).
func Configure(cwd, home string) (applied bool, err error) {
	candidates := []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(cwd, ".claude", "settings.json"),
		filepath.Join(cwd, ".claude", "settings.local.json"),
	}
	for _, p := range candidates {
		if hasStatusLine(p) {
			return false, nil
		}
	}
	local := filepath.Join(cwd, ".claude", "settings.local.json")
	if err := mergeStatusLine(local, "node "+installPath()); err != nil {
		return false, err
	}
	if err := ensureGitignore(filepath.Join(cwd, ".claude")); err != nil {
		return true, err
	}
	return true, nil
}

// ensureGitignore makes sure <claudeDir>/.gitignore ignores the personal files
// fleet writes there, so a user never accidentally commits a machine-specific
// status-line path. Appends only the patterns that are missing; never clobbers.
func ensureGitignore(claudeDir string) error {
	path := filepath.Join(claudeDir, ".gitignore")
	want := []string{"settings.local.json", "settings.local.json.bak"}
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return err
	}
	lines := map[string]bool{}
	for _, l := range strings.Split(existing, "\n") {
		lines[strings.TrimSpace(l)] = true
	}
	add := ""
	for _, p := range want {
		if !lines[p] {
			add += p + "\n"
		}
	}
	if add == "" {
		return nil
	}
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		add = "\n" + add
	}
	return os.WriteFile(path, []byte(existing+add), 0644)
}
