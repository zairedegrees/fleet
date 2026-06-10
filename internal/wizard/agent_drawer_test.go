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
	_, saved := driveDrawer(t, d, enter, enter, enter, enter, enter)
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

// Without toggling, a new agent saves with AutoTalk=false.
func TestDrawerCreateDefaultsAutoTalkOff(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	msgs := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("scout")},
		enter, enter, enter, enter, enter,
	}
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg")
	}
	if saved.Agent.AutoTalk {
		t.Error("new agent must default to AutoTalk=false")
	}
}
