package wizard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

// A relay failure while loading agents must be surfaced, not swallowed into an
// empty list with zero feedback.
func TestWizardCapturesRelayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"relay down"}}`)
	}))
	defer srv.Close()

	m := newWizardModel(relay.NewClient(srv.URL))
	updated, _ := m.Update(ProjectSelectedMsg{Name: "proj", Path: "/tmp"})
	wm := updated.(wizardModel)
	if !strings.Contains(wm.status, "relay") {
		t.Errorf("expected the relay error captured in status, got: %q", wm.status)
	}
}

// The captured status must actually render so the user sees it.
func TestWizardViewShowsStatus(t *testing.T) {
	m := newWizardModel(nil)
	m.status = "relay unreachable"
	if !strings.Contains(m.View(), "relay unreachable") {
		t.Errorf("View should surface the status message; got:\n%s", m.View())
	}
}

// The relay URL set in the wizard must flow into the Result config and survive
// a TOML save/load round-trip — that is what --status and dispatch read back.
func TestWizardRelayURLRoundTrip(t *testing.T) {
	m := newWizardModel(nil)
	m.agents.SetAgents([]agentItem{
		{agent: config.AgentConfig{Name: "dev", Color: "green", Role: "Lead"}, enabled: true},
	})
	m.project.projName = "roundtrip"
	m.project.pathInput.SetValue("/tmp")
	m.project.relayInput.SetValue("http://relay.example:9000/mcp")
	m.launching = true

	res := m.Result()
	if res == nil {
		t.Fatal("expected a wizard result")
	}
	if res.Config.Project.RelayURL != "http://relay.example:9000/mcp" {
		t.Fatalf("Result config must carry the wizard relay URL, got %q", res.Config.Project.RelayURL)
	}

	path := filepath.Join(t.TempDir(), "roundtrip.toml")
	if err := config.Save(path, res.Config); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Project.RelayURL != "http://relay.example:9000/mcp" {
		t.Errorf("relay URL must survive the TOML round-trip, got %q", loaded.Project.RelayURL)
	}
}

// An untouched relay field still yields a usable config: the default URL.
func TestWizardRelayURLDefaultsInResult(t *testing.T) {
	m := newWizardModel(nil)
	m.agents.SetAgents([]agentItem{
		{agent: config.AgentConfig{Name: "dev", Color: "green", Role: "Lead"}, enabled: true},
	})
	m.project.projName = "p"
	m.project.pathInput.SetValue("/tmp")
	m.launching = true

	res := m.Result()
	if res == nil {
		t.Fatal("expected a wizard result")
	}
	if res.Config.Project.RelayURL != config.DefaultRelayURL {
		t.Errorf("untouched field must yield the default URL, got %q", res.Config.Project.RelayURL)
	}
}

// Loading an existing project must prefill its saved relay URL so re-saving
// does not silently reset it to the default.
func TestWizardLoadedProjectPrefillsRelayURL(t *testing.T) {
	m := newWizardModel(nil)
	updated, _ := m.Update(ProjectLoadedMsg{Config: &config.FleetConfig{
		Project: config.ProjectConfig{Name: "p", Cwd: "/tmp", RelayURL: "http://saved.example:7000/mcp"},
	}})
	wm := updated.(wizardModel)
	if got := wm.project.RelayURL(); got != "http://saved.example:7000/mcp" {
		t.Errorf("loaded project must prefill its relay URL, got %q", got)
	}
}

// Reopening a project must leave a discoverable way to edit its saved relay URL:
// tab to the settings hub, move to the Relay row, open it, edit, confirm.
func TestWizardLoadedProjectCanEditRelayURL(t *testing.T) {
	m := newWizardModel(nil)
	updated, _ := m.Update(ProjectLoadedMsg{Config: &config.FleetConfig{
		Project: config.ProjectConfig{Name: "p", Cwd: "/tmp", RelayURL: "http://saved.example:7000/mcp"},
	}})
	wm := updated.(wizardModel)

	// Loaded projects land on the agents panel; tab back to the settings hub.
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyTab})
	wm = updated.(wizardModel)
	if wm.activePanel != panelLeft || wm.project.focus != focusSettings {
		t.Fatalf("tab must land on the settings hub, got panel=%v focus=%v", wm.activePanel, wm.project.focus)
	}

	// Move to the Relay row and open it.
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // Path -> Relay
	wm = updated.(wizardModel)
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	wm = updated.(wizardModel)
	if wm.project.focus != focusRelayURL {
		t.Fatalf("enter on the Relay row must open the relay field, got %v", wm.project.focus)
	}
	if got := wm.project.relayInput.Value(); got != "http://saved.example:7000/mcp" {
		t.Fatalf("relay field must keep the saved URL, got %q", got)
	}

	wm.project.relayInput.SetValue("http://edited.example:7100/mcp")
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	wm = updated.(wizardModel)
	if wm.project.focus != focusSettings {
		t.Fatalf("confirming the edit must return to the settings hub, got %v", wm.project.focus)
	}
	if got := wm.project.RelayURL(); got != "http://edited.example:7100/mcp" {
		t.Errorf("edited relay URL must be kept, got %q", got)
	}
}

// Pins focusRelayURL in the isTextInput routing: a shortcut letter typed into
// the focused relay URL field must land in the input, not quit the wizard.
func TestWizardTypingIntoRelayURLFieldIsNotShortcut(t *testing.T) {
	m := newWizardModel(nil)
	m.project.focus = focusRelayURL
	m.project.relayInput.SetValue("")
	m.project.relayInput.Focus()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	wm := updated.(wizardModel)
	if wm.quitting {
		t.Fatal("'q' typed into the relay URL field must not quit the wizard")
	}
	if got := wm.project.relayInput.Value(); got != "q" {
		t.Errorf("typed rune must land in the relay URL input, got %q", got)
	}
}

// Only esc was repurposed as step-back on the presets focus — q still quits.
func TestWizardPresetsQStillQuits(t *testing.T) {
	m := newWizardModel(nil)
	m.project.ready = true
	m.project.focus = focusPresets

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	wm := updated.(wizardModel)
	if !wm.quitting {
		t.Error("'q' on the presets step must quit the wizard")
	}
}

// Toggling auto-talk in the drawer must write back to the real agent entry in
// the model (not a value-receiver copy), flow into the Result config, and
// survive a TOML save/load round-trip.
func TestWizardAutoTalkRoundTrip(t *testing.T) {
	m := newWizardModel(nil)
	m.agents.SetAgents([]agentItem{
		{agent: config.AgentConfig{Name: "dev", Color: "green", Role: "Lead"}, enabled: true},
	})

	step := func(msg tea.Msg) {
		t.Helper()
		updated, cmd := m.Update(msg)
		m = updated.(wizardModel)
		if cmd != nil {
			if next := cmd(); next != nil {
				updated, _ = m.Update(next)
				m = updated.(wizardModel)
			}
		}
	}

	step(EditAgentMsg{Index: 0})
	if !m.drawerOpen {
		t.Fatal("drawer should be open after EditAgentMsg")
	}
	// Computed navigation from the drawerFields table so adding drawer fields
	// never silently breaks this round-trip (uses tab, which advances every kind).
	tab := tea.KeyMsg{Type: tea.KeyTab}
	for range fieldIndex(dfAutoTalk) {
		step(tab) // name -> ... -> auto-talk
	}
	step(tea.KeyMsg{Type: tea.KeyRight}) // off -> on
	for range len(drawerFields) - fieldIndex(dfAutoTalk) {
		step(tab) // auto-talk -> ... -> save
	}

	if m.drawerOpen {
		t.Fatal("drawer should be closed after save")
	}
	if !m.agents.items[0].agent.AutoTalk {
		t.Fatal("toggle must write AutoTalk=true back to the model's agent entry")
	}

	m.project.projName = "roundtrip"
	m.project.pathInput.SetValue("/tmp")
	m.launching = true
	res := m.Result()
	if res == nil {
		t.Fatal("expected a wizard result")
	}
	if !res.Config.Agents[0].AutoTalk {
		t.Fatal("Result config must carry AutoTalk=true")
	}

	path := filepath.Join(t.TempDir(), "roundtrip.toml")
	if err := config.Save(path, res.Config); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.Agents[0].AutoTalk {
		t.Error("AutoTalk=true must survive the TOML save/load round-trip")
	}
}

// P on the agents panel toggles fleet-wide skip-all autonomy; off by default.
func TestWizardAutonomyToggle(t *testing.T) {
	m := newWizardModel(nil)
	m.activePanel = panelRight
	if m.skipPerms {
		t.Fatal("autonomy must be OFF by default")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	wm := updated.(wizardModel)
	if !wm.skipPerms {
		t.Fatal("P must toggle skip-all autonomy ON")
	}
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	wm = updated.(wizardModel)
	if wm.skipPerms {
		t.Fatal("P must toggle skip-all autonomy back OFF")
	}
}

// Skip-all OFF → no claude flags; ON → --dangerously-skip-permissions, and it
// survives the TOML round-trip so --last relaunches with the same posture.
func TestWizardAutonomyFlagsInResult(t *testing.T) {
	build := func(skip bool) *config.FleetConfig {
		m := newWizardModel(nil)
		m.agents.SetAgents([]agentItem{
			{agent: config.AgentConfig{Name: "dev", Color: "green", Role: "Lead"}, enabled: true},
		})
		m.project.projName = "p"
		m.project.pathInput.SetValue("/tmp")
		m.skipPerms = skip
		m.launching = true
		res := m.Result()
		if res == nil {
			t.Fatal("expected a wizard result")
		}
		return res.Config
	}

	if flags := build(false).Claude.Flags; len(flags) != 0 {
		t.Errorf("autonomy OFF must leave claude flags empty, got %v", flags)
	}

	cfg := build(true)
	if len(cfg.Claude.Flags) != 1 || cfg.Claude.Flags[0] != "--dangerously-skip-permissions" {
		t.Fatalf("autonomy ON must set the skip-permissions flag, got %v", cfg.Claude.Flags)
	}

	path := filepath.Join(t.TempDir(), "autonomy.toml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Claude.Flags) != 1 || loaded.Claude.Flags[0] != "--dangerously-skip-permissions" {
		t.Errorf("skip-permissions flag must survive the TOML round-trip, got %v", loaded.Claude.Flags)
	}
}

// P must not toggle autonomy while a text input has focus — it's a plain
// character typed into the path/relay field, not a shortcut.
func TestWizardAutonomyNotToggledInTextInput(t *testing.T) {
	m := newWizardModel(nil)
	m.activePanel = panelLeft
	m.project.focus = focusPath
	m.project.pathInput.Focus()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	wm := updated.(wizardModel)
	if wm.skipPerms {
		t.Error("P typed into the path field must not toggle autonomy")
	}
}

// The autonomy posture is visible in the rendered view.
func TestWizardViewShowsAutonomy(t *testing.T) {
	m := newWizardModel(nil)
	m.activePanel = panelRight
	if !strings.Contains(m.View(), "Autonomy") {
		t.Errorf("View must show the autonomy posture; got:\n%s", m.View())
	}
	m.skipPerms = true
	if !strings.Contains(m.View(), "SKIP-ALL") {
		t.Errorf("View must flag SKIP-ALL when autonomy is on; got:\n%s", m.View())
	}
}

// A relay agent whose name/role carries terminal control sequences must be
// neutralized before it reaches the wizard's rendered items — the relay is
// untrusted (any agent can register a name).
func TestWizardSanitizesRelayAgentNames(t *testing.T) {
	evil := "ghost" + string(rune(0x1b)) + "]0;pwned" + string(rune(0x07))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		inner, _ := json.Marshal(map[string]any{
			"agents": []relay.Agent{{Name: evil, Role: evil, ProfileSlug: "ghost", Status: "inactive"}},
		})
		text, _ := json.Marshal(string(inner))
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":%s}]}}`, text)
	}))
	defer srv.Close()

	m := newWizardModel(relay.NewClient(srv.URL))
	updated, _ := m.Update(ProjectSelectedMsg{Name: "proj", Path: "/tmp"})
	wm := updated.(wizardModel)

	if len(wm.agents.items) == 0 {
		t.Fatal("expected the relay agent to be loaded into wizard items")
	}
	got := wm.agents.items[0].agent
	if strings.ContainsRune(got.Name, 0x1b) || strings.ContainsRune(got.Name, 0x07) ||
		strings.ContainsRune(got.Role, 0x1b) || strings.ContainsRune(got.Role, 0x07) {
		t.Errorf("wizard kept control chars: name=%q role=%q", got.Name, got.Role)
	}
	if !strings.HasPrefix(got.Name, "ghost") {
		t.Errorf("expected sanitized name preserving printable text, got %q", got.Name)
	}
}

// Loading a project lands on the settings hub (left) while showing agents (right).
func TestWizardLoadedProjectLandsOnSettings(t *testing.T) {
	m := newWizardModel(nil)
	updated, _ := m.Update(ProjectLoadedMsg{Config: &config.FleetConfig{
		Project: config.ProjectConfig{Name: "p", Cwd: "/tmp", RelayURL: "http://saved:7000/mcp"},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Lead"}},
	}})
	wm := updated.(wizardModel)
	if wm.project.focus != focusSettings {
		t.Fatalf("loaded project must land on the settings hub, got %v", wm.project.focus)
	}
	if wm.activePanel != panelRight {
		t.Fatalf("loaded project must show the agents panel, got %v", wm.activePanel)
	}
	if !wm.project.ready {
		t.Fatal("loaded project must be ready")
	}
}

// The settings hub help line points the user to the agents panel.
func TestWizardSettingsHelp(t *testing.T) {
	m := newWizardModel(nil)
	m.project.ready = true
	m.activePanel = panelLeft
	m.project.focus = focusSettings
	if !strings.Contains(m.View(), "tab agents") {
		t.Errorf("settings help must mention tab to agents; got:\n%s", m.View())
	}
}

// Esc walks up one level and only quits from the project list — never a surprise quit.
func TestWizardEscLadder(t *testing.T) {
	m := newWizardModel(nil)
	updated, _ := m.Update(ProjectLoadedMsg{Config: &config.FleetConfig{
		Project: config.ProjectConfig{Name: "p", Cwd: "/tmp"},
	}})
	wm := updated.(wizardModel)

	// Agents panel: esc -> settings hub (left), not quit.
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	wm = updated.(wizardModel)
	if wm.quitting || wm.activePanel != panelLeft || wm.project.focus != focusSettings {
		t.Fatalf("esc on agents must go to settings, got quit=%v panel=%v focus=%v", wm.quitting, wm.activePanel, wm.project.focus)
	}

	// Settings hub: esc -> project list.
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	wm = updated.(wizardModel)
	if wm.quitting || wm.project.focus != focusProjectList {
		t.Fatalf("esc on settings must go to the project list, got quit=%v focus=%v", wm.quitting, wm.project.focus)
	}

	// Project list: esc -> quit.
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	wm = updated.(wizardModel)
	if !wm.quitting {
		t.Fatal("esc on the project list must quit")
	}
}
