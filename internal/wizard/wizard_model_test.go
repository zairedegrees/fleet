package wizard

import (
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
	tab := tea.KeyMsg{Type: tea.KeyTab}
	step(tab)                            // name -> role
	step(tab)                            // role -> color
	step(tab)                            // color -> reports-to
	step(tab)                            // reports-to -> auto-talk
	step(tea.KeyMsg{Type: tea.KeyRight}) // off -> on
	step(tea.KeyMsg{Type: tea.KeyEnter}) // auto-talk -> executive
	step(tea.KeyMsg{Type: tea.KeyEnter}) // save -> DrawerSaveMsg

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
