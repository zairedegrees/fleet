package wizard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zairedegrees/fleet/internal/config"
)

// driveDrawer feeds key messages through the drawer's Update loop and returns
// the final drawer plus the DrawerSaveMsg if a save was emitted.
func driveDrawer(t *testing.T, d agentDrawer, msgs ...tea.Msg) (agentDrawer, *DrawerSaveMsg) {
	t.Helper()
	var saved *DrawerSaveMsg
	for _, msg := range msgs {
		var cmd tea.Cmd
		d, cmd = d.Update(msg)
		if cmd != nil {
			if m, ok := cmd().(DrawerSaveMsg); ok {
				saved = &m
			}
		}
	}
	return d, saved
}

// Editing an agent and saving without touching the auto-talk field must
// preserve its existing AutoTalk value — not silently reset it to false.
func TestDrawerEditPreservesAutoTalk(t *testing.T) {
	d := newAgentDrawer()
	d.OpenEdit(0, config.AgentConfig{
		Name: "dev", Color: "green", Role: "Lead", AutoTalk: true,
	}, []string{"dev"})

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	_, saved := driveDrawer(t, d, enter, enter, enter, enter, enter, enter)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg after entering through all fields")
	}
	if !saved.Agent.AutoTalk {
		t.Error("editing an agent must preserve AutoTalk=true, got false")
	}
}

// A new agent defaults to AutoTalk=false; toggling the auto-talk field in the
// drawer must flip it to true in the saved agent.
func TestDrawerToggleAutoTalk(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	tab := tea.KeyMsg{Type: tea.KeyTab}
	msgs := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("scout")},
		tab,                            // name -> role
		tab,                            // role -> color
		tab,                            // color -> reports-to
		tab,                            // reports-to -> auto-talk
		tea.KeyMsg{Type: tea.KeyRight}, // off -> on
		tea.KeyMsg{Type: tea.KeyEnter}, // auto-talk -> executive
		tea.KeyMsg{Type: tea.KeyEnter}, // save
	}
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg after toggling auto-talk and saving")
	}
	if !saved.Agent.AutoTalk {
		t.Error("toggled auto-talk must save AutoTalk=true, got false")
	}
}

// Pins OpenEdit prefilling the executive toggle from the agent being edited:
// changing an executive's role must not silently drop IsExecutive. Note this
// does NOT pin the save() base-capture — every AgentConfig field is explicitly
// drawer-managed today, so dropping the base is an equivalent mutant no test
// can kill (see the base field comment in agent_drawer.go).
func TestDrawerEditPreservesIsExecutive(t *testing.T) {
	d := newAgentDrawer()
	d.OpenEdit(0, config.AgentConfig{
		Name: "boss", Color: "green", Role: "Lead", IsExecutive: true,
	}, []string{"boss"})

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	msgs := []tea.Msg{
		enter, // name -> role
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" v2")}, // edit the role
		enter, enter, enter, enter, enter, // through remaining fields -> save
	}
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg after entering through all fields")
	}
	if saved.Agent.Role != "Lead v2" {
		t.Errorf("role edit must be saved, got %q", saved.Agent.Role)
	}
	if !saved.Agent.IsExecutive {
		t.Error("editing an executive agent must preserve IsExecutive=true, got false")
	}
}

// A new agent defaults to IsExecutive=false; toggling the executive field in
// the drawer must flip it to true in the saved agent — without touching AutoTalk.
func TestDrawerToggleExecutive(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	tab := tea.KeyMsg{Type: tea.KeyTab}
	msgs := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("boss")},
		tab,                            // name -> role
		tab,                            // role -> color
		tab,                            // color -> reports-to
		tab,                            // reports-to -> auto-talk
		tab,                            // auto-talk -> executive
		tea.KeyMsg{Type: tea.KeyRight}, // off -> on
		tea.KeyMsg{Type: tea.KeyEnter}, // save
	}
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg after toggling executive and saving")
	}
	if !saved.Agent.IsExecutive {
		t.Error("toggled executive must save IsExecutive=true, got false")
	}
	if saved.Agent.AutoTalk {
		t.Error("executive toggle must not flip AutoTalk")
	}
}

// Without toggling, a new agent saves with IsExecutive=false.
func TestDrawerCreateDefaultsExecutiveOff(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	msgs := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("scout")},
		enter, enter, enter, enter, enter, enter,
	}
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg")
	}
	if saved.Agent.IsExecutive {
		t.Error("new agent must default to IsExecutive=false")
	}
}

// Without toggling, a new agent saves with AutoTalk=false.
func TestDrawerCreateDefaultsAutoTalkOff(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	msgs := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("scout")},
		enter, enter, enter, enter, enter, enter,
	}
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg")
	}
	if saved.Agent.AutoTalk {
		t.Error("new agent must default to AutoTalk=false")
	}
}
