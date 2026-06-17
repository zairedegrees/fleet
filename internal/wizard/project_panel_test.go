package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zairedegrees/fleet/internal/config"
)

// deriveProjectName must produce a name that survives config.Validate(), so a
// folder like "site.com" or "My App" yields a safe project name instead of one
// that later fails launch (or worse, injects into the generated shell scripts).
func TestDeriveProjectName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/Users/x/site.com", "site-com"},
		{"/Users/x/My App", "My-App"},
		{"/home/u/clean-name", "clean-name"},
	}
	for _, tc := range tests {
		if got := deriveProjectName(tc.path); got != tc.want {
			t.Errorf("deriveProjectName(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// The relay URL field defaults to the standard relay so a user who never
// touches it gets the same behavior as before the field existed.
func TestProjectPanelRelayURLDefault(t *testing.T) {
	p := newProjectPanel()
	if got := p.RelayURL(); got != config.DefaultRelayURL {
		t.Errorf("RelayURL() = %q, want default %q", got, config.DefaultRelayURL)
	}
}

// Confirming the path leads to the editable relay URL field; confirming that
// leads to the presets, with the panel ready and the custom URL kept.
func TestProjectPanelPathThenRelayURLFlow(t *testing.T) {
	p := newProjectPanel()
	p.focus = focusPath
	p.pathInput.SetValue(t.TempDir())

	p, _ = p.updatePathInput(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focus != focusRelayURL {
		t.Fatalf("path enter must focus the relay URL field, got focus %v", p.focus)
	}
	if p.ready {
		t.Error("panel must not be ready before the relay URL is confirmed")
	}

	p.relayInput.SetValue("http://custom:9999/mcp")
	p, _ = p.updateRelayInput(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focus != focusSettings || !p.ready {
		t.Fatalf("relay enter must confirm and move to settings, got focus=%v ready=%v", p.focus, p.ready)
	}
	if got := p.RelayURL(); got != "http://custom:9999/mcp" {
		t.Errorf("RelayURL() = %q, want the entered URL", got)
	}
}

// Clearing the field is not an error — it falls back to the default URL,
// mirroring how an empty relay_url behaves everywhere else.
func TestProjectPanelRelayURLEmptyFallsBackToDefault(t *testing.T) {
	p := newProjectPanel()
	p.focus = focusRelayURL
	p.relayInput.SetValue("")
	p, _ = p.updateRelayInput(tea.KeyMsg{Type: tea.KeyEnter})
	if got := p.RelayURL(); got != config.DefaultRelayURL {
		t.Errorf("empty relay URL must fall back to default, got %q", got)
	}
}

// A malformed relay URL must be rejected on submit — error visible in the
// panel, step unchanged — instead of aborting the launch after the wizard has
// exited, when the whole typed config is already lost.
func TestProjectPanelRelayURLInvalidRejectedOnSubmit(t *testing.T) {
	invalid := []string{
		"localhost:8090/mcp", // url.Parse reads "localhost" as the scheme
		"htp://localhost:8090/mcp",
		"http://",
		"not a url",
	}
	for _, raw := range invalid {
		p := newProjectPanel()
		p.focus = focusRelayURL
		p.relayInput.SetValue(raw)
		p, _ = p.updateRelayInput(tea.KeyMsg{Type: tea.KeyEnter})
		if p.focus != focusRelayURL {
			t.Errorf("%q: submit must stay on the relay URL step, got focus %v", raw, p.focus)
		}
		if p.ready {
			t.Errorf("%q: panel must not become ready with an invalid relay URL", raw)
		}
		if !strings.Contains(p.View(true), "relay URL") {
			t.Errorf("%q: the panel must show the validation error, got:\n%s", raw, p.View(true))
		}
	}
}

// After a rejected submit, correcting the URL must clear the error and proceed.
func TestProjectPanelRelayURLValidAfterInvalidProceeds(t *testing.T) {
	p := newProjectPanel()
	p.focus = focusRelayURL
	p.relayInput.SetValue("htp://oops")
	p, _ = p.updateRelayInput(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focus != focusRelayURL || p.ready {
		t.Fatalf("invalid URL must keep the step, got focus=%v ready=%v", p.focus, p.ready)
	}

	p.relayInput.SetValue("http://localhost:9999/mcp")
	p, _ = p.updateRelayInput(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focus != focusSettings || !p.ready {
		t.Fatalf("corrected URL must confirm and move to settings, got focus=%v ready=%v", p.focus, p.ready)
	}
	if strings.Contains(p.View(true), "must start with") {
		t.Error("a successful submit must clear the validation error")
	}
}

// Esc from the relay field goes back to the path input, matching the
// esc-from-path pattern.
func TestProjectPanelRelayURLEscGoesBackToPath(t *testing.T) {
	p := newProjectPanel()
	p.focus = focusRelayURL
	p, _ = p.updateRelayInput(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focus != focusPath {
		t.Errorf("esc must return to the path input, got focus %v", p.focus)
	}
}

// Projects are listed most-recently-used first, by config file mtime; a
// config-less project (no mtime) sinks to the bottom.
func TestDiscoverProjectsSortedByRecency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".fleet", "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, age time.Duration) {
		p := filepath.Join(cfgDir, name+".toml")
		if err := config.Save(p, &config.FleetConfig{
			Project: config.ProjectConfig{Name: name, Cwd: "/tmp/" + name},
		}); err != nil {
			t.Fatal(err)
		}
		mt := time.Now().Add(-age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	write("old", 3*time.Hour)
	write("newest", 1*time.Minute)
	write("middle", 1*time.Hour)

	// A config-less project (projects file only, no mtime) must sink below all
	// mtime-backed entries.
	if err := os.WriteFile(filepath.Join(home, ".fleet", "projects"), []byte("orphan\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, p := range discoverProjects() {
		names = append(names, p.name)
	}
	if got := strings.Join(names, ","); got != "newest,middle,old,orphan" {
		t.Errorf("recency order = %q, want newest,middle,old,orphan", got)
	}
}

// In the settings hub, j/k move the cursor (clamped) and enter dives into the
// focused field's editor.
func TestProjectPanelSettingsNavigation(t *testing.T) {
	p := newProjectPanel()
	p.ready = true
	p.focus = focusSettings
	p.settingsCursor = 0

	j := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	p, _ = p.updateSettings(j)
	if p.settingsCursor != 1 {
		t.Fatalf("j must move to Relay (1), got %d", p.settingsCursor)
	}
	p, _ = p.updateSettings(j)
	p, _ = p.updateSettings(j) // clamp at 2
	if p.settingsCursor != 2 {
		t.Fatalf("cursor must clamp at Team (2), got %d", p.settingsCursor)
	}
	p, _ = p.updateSettings(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focus != focusPresets {
		t.Fatalf("enter on Team must open the preset chooser, got focus %v", p.focus)
	}
}

func TestProjectPanelSettingsEntersEditors(t *testing.T) {
	p := newProjectPanel()
	p.ready = true
	p.focus = focusSettings

	p.settingsCursor = 0
	pp, _ := p.updateSettings(tea.KeyMsg{Type: tea.KeyEnter})
	if pp.focus != focusPath {
		t.Fatalf("enter on Path must open the path editor, got %v", pp.focus)
	}

	p.settingsCursor = 1
	pp, _ = p.updateSettings(tea.KeyMsg{Type: tea.KeyEnter})
	if pp.focus != focusRelayURL {
		t.Fatalf("enter on Relay must open the relay editor, got %v", pp.focus)
	}
}

// Editing a field from the hub (ready) returns to the hub — not the new-project
// linear chain.
func TestProjectPanelHubEditReturnsToSettings(t *testing.T) {
	p := newProjectPanel()
	p.ready = true

	p.focus = focusPath
	p.pathInput.SetValue(t.TempDir())
	p, _ = p.updatePathInput(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focus != focusSettings {
		t.Fatalf("hub path-edit enter must return to settings, got %v", p.focus)
	}

	p.focus = focusRelayURL
	p.relayInput.SetValue("http://x:1/mcp")
	p, _ = p.updateRelayInput(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focus != focusSettings {
		t.Fatalf("hub relay-edit enter must return to settings, got %v", p.focus)
	}

	p.focus = focusRelayURL
	p, _ = p.updateRelayInput(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focus != focusSettings {
		t.Fatalf("hub relay-edit esc must return to settings, got %v", p.focus)
	}
}

// The cursor lands on the last-launched project (last.toml target), even when
// it is not the most recent by mtime.
func TestNewProjectPanelPreselectsLastProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".fleet", "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, age time.Duration) string {
		p := filepath.Join(cfgDir, name+".toml")
		if err := config.Save(p, &config.FleetConfig{
			Project: config.ProjectConfig{Name: name, Cwd: "/tmp/" + name},
		}); err != nil {
			t.Fatal(err)
		}
		mt := time.Now().Add(-age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("alpha", 1*time.Minute) // newest by mtime
	betaPath := write("beta", 2*time.Hour)
	write("gamma", 3*time.Hour)

	// last.toml -> beta proves preselection follows last.toml, not index 0.
	if err := os.Symlink(betaPath, filepath.Join(home, ".fleet", "last.toml")); err != nil {
		t.Fatal(err)
	}

	p := newProjectPanel()
	if p.projectCursor >= len(p.existingProjects) {
		t.Fatalf("cursor %d out of range (%d projects)", p.projectCursor, len(p.existingProjects))
	}
	if got := p.existingProjects[p.projectCursor].name; got != "beta" {
		t.Errorf("preselected %q, want beta (last.toml target)", got)
	}
}
