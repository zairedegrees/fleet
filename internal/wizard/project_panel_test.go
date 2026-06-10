package wizard

import (
	"testing"

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
	if p.focus != focusPresets || !p.ready {
		t.Fatalf("relay enter must confirm and move to presets, got focus=%v ready=%v", p.focus, p.ready)
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
