package wizard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zairedegrees/fleet/internal/config"
)

// --- navigation helpers -----------------------------------------------------
//
// Drawer navigation is COMPUTED from the drawerFields table, never hard-counted,
// so adding a field never silently breaks these tests. Navigation uses TAB,
// which advances every field kind (a textarea swallows Enter as a newline, so
// Enter is not a universal advance key — see TestDrawerEnterAdvancesField).

var (
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyRight = tea.KeyMsg{Type: tea.KeyRight}
)

func typeRunes(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func rightN(n int) []tea.Msg {
	msgs := make([]tea.Msg, n)
	for i := range msgs {
		msgs[i] = keyRight
	}
	return msgs
}

func tabN(n int) []tea.Msg {
	msgs := make([]tea.Msg, n)
	for i := range msgs {
		msgs[i] = keyTab
	}
	return msgs
}

// tabsTo returns the tabs needed to move focus from the first field onto id.
func tabsTo(id drawerField) []tea.Msg { return tabN(fieldIndex(id)) }

// tabsFrom returns the tabs needed to walk from id past the last field, which
// triggers the save.
func tabsFrom(id drawerField) []tea.Msg { return tabN(len(drawerFields) - fieldIndex(id)) }

// tabsToSave returns the tabs needed to walk from the first field to the save.
func tabsToSave() []tea.Msg { return tabN(len(drawerFields)) }

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

// concat flattens message groups into one ordered slice.
func concat(groups ...[]tea.Msg) []tea.Msg {
	var out []tea.Msg
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// --- characterization (locked before/after the table refactor) --------------

// Locks the drawer's field navigation order: tab must walk Name → Role → Color
// → Reports → Auto-talk → Executive.
func TestDrawerFieldNavigationOrder(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)
	d, _ = driveDrawer(t, d, typeRunes("x"))

	want := []drawerField{dfRole, dfColor, dfReportsTo, dfModel, dfPermission, dfAutoTalk, dfExecutive}
	for i, w := range want {
		d, _ = driveDrawer(t, d, keyTab)
		if d.field != w {
			t.Errorf("after %d tab(s), field = %d, want %d", i+1, d.field, w)
		}
	}
}

// Locks that View renders every field's label, so the refactor's render loop
// can't silently drop one.
func TestDrawerViewRendersAllFields(t *testing.T) {
	d := newAgentDrawer()
	d.OpenEdit(0, config.AgentConfig{Name: "dev", Color: "green", Role: "Lead"}, []string{"dev"})
	v := d.View()
	for _, label := range []string{"Name:", "Role:", "Color:", "Reports to:", "Model:", "Permission:", "Auto-talk:", "Executive:"} {
		if !strings.Contains(v, label) {
			t.Errorf("drawer View is missing field label %q", label)
		}
	}
}

// Editing an agent and saving without touching Model/Permission preserves both
// (the value-receiver write-back + base-capture path that AutoTalk also rides).
func TestDrawerEditPreservesModelAndPermission(t *testing.T) {
	d := newAgentDrawer()
	d.OpenEdit(0, config.AgentConfig{
		Name: "auditor", Color: "red", Role: "Review", Model: "opus", PermissionMode: "plan",
	}, []string{"auditor"})

	_, saved := driveDrawer(t, d, tabsToSave()...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg")
	}
	if saved.Agent.Model != "opus" {
		t.Errorf("model must be preserved, got %q", saved.Agent.Model)
	}
	if saved.Agent.PermissionMode != "plan" {
		t.Errorf("permission_mode must be preserved, got %q", saved.Agent.PermissionMode)
	}
}

// Selecting Model and Permission in the drawer writes them to the saved agent.
func TestDrawerSetModelAndPermission(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	// modelOpts:    inherit, opus, sonnet, haiku           → 2 rights = sonnet
	// permModeOpts: inherit, default, acceptEdits, plan, … → 3 rights = plan
	flow := concat(
		[]tea.Msg{typeRunes("dev")},
		tabsTo(dfModel),
		rightN(2), // inherit -> opus -> sonnet
		tabN(fieldIndex(dfPermission)-fieldIndex(dfModel)), // model -> permission
		rightN(3),              // inherit -> default -> acceptEdits -> plan
		tabsFrom(dfPermission), // permission -> ... -> save
	)
	_, saved := driveDrawer(t, d, flow...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg")
	}
	if saved.Agent.Model != "sonnet" {
		t.Errorf("selected model = %q, want sonnet", saved.Agent.Model)
	}
	if saved.Agent.PermissionMode != "plan" {
		t.Errorf("selected permission = %q, want plan", saved.Agent.PermissionMode)
	}
}

// Persona/Skills/Tools are not editable in the drawer, so editing an agent must
// carry them through untouched via the base capture (now observable behavior).
func TestDrawerEditPreservesUnmanagedFields(t *testing.T) {
	d := newAgentDrawer()
	d.OpenEdit(0, config.AgentConfig{
		Name: "auditor", Color: "red", Role: "Review",
		Persona: "You are the auditor.\nGo quiet on an empty inbox.",
		Skills:  []string{"code-review"},
		Tools:   []string{"Read", "Grep"},
	}, []string{"auditor"})

	_, saved := driveDrawer(t, d, tabsToSave()...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg")
	}
	if saved.Agent.Persona == "" || saved.Agent.Skills == nil || saved.Agent.Tools == nil {
		t.Errorf("editing must preserve unmanaged fields, got %+v", saved.Agent)
	}
}

// An untouched new agent saves with inherit (empty) Model and PermissionMode.
func TestDrawerCreateDefaultsModelPermissionInherit(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	msgs := concat([]tea.Msg{typeRunes("dev")}, tabsToSave())
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg")
	}
	if saved.Agent.Model != "" {
		t.Errorf("default model must be inherit (empty), got %q", saved.Agent.Model)
	}
	if saved.Agent.PermissionMode != "" {
		t.Errorf("default permission must be inherit (empty), got %q", saved.Agent.PermissionMode)
	}
}

// Enter advances a focused TEXT field (the universal-advance key for everything
// except a textarea). Pins enter-navigation without a full traversal, so it
// stays valid once a textarea field exists.
func TestDrawerEnterAdvancesField(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)
	d, _ = driveDrawer(t, d, typeRunes("scout"), keyEnter)
	if d.field != dfRole {
		t.Errorf("enter on a non-empty name field must advance to role, got field %d", d.field)
	}
}

// --- save / preserve behavior (computed navigation) -------------------------

// Editing an agent and saving without touching the auto-talk field must
// preserve its existing AutoTalk value — not silently reset it to false.
func TestDrawerEditPreservesAutoTalk(t *testing.T) {
	d := newAgentDrawer()
	d.OpenEdit(0, config.AgentConfig{Name: "dev", Color: "green", Role: "Lead", AutoTalk: true}, []string{"dev"})

	_, saved := driveDrawer(t, d, tabsToSave()...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg after tabbing through all fields")
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

	msgs := concat(
		[]tea.Msg{typeRunes("scout")},
		tabsTo(dfAutoTalk),
		[]tea.Msg{keyRight}, // off -> on
		tabsFrom(dfAutoTalk),
	)
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg after toggling auto-talk and saving")
	}
	if !saved.Agent.AutoTalk {
		t.Error("toggled auto-talk must save AutoTalk=true, got false")
	}
}

// Pins OpenEdit prefilling the executive toggle and that editing the role does
// not silently drop IsExecutive.
func TestDrawerEditPreservesIsExecutive(t *testing.T) {
	d := newAgentDrawer()
	d.OpenEdit(0, config.AgentConfig{Name: "boss", Color: "green", Role: "Lead", IsExecutive: true}, []string{"boss"})

	msgs := concat(
		[]tea.Msg{keyTab},           // name -> role
		[]tea.Msg{typeRunes(" v2")}, // edit the role
		tabsFrom(dfRole),            // role -> ... -> save
	)
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg after tabbing through all fields")
	}
	if saved.Agent.Role != "Lead v2" {
		t.Errorf("role edit must be saved, got %q", saved.Agent.Role)
	}
	if !saved.Agent.IsExecutive {
		t.Error("editing an executive agent must preserve IsExecutive=true, got false")
	}
}

// A new agent defaults to IsExecutive=false; toggling the executive field must
// flip it to true — without touching AutoTalk.
func TestDrawerToggleExecutive(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	msgs := concat(
		[]tea.Msg{typeRunes("boss")},
		tabsTo(dfExecutive),
		[]tea.Msg{keyRight}, // off -> on
		tabsFrom(dfExecutive),
	)
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

// Without toggling, a new agent saves with IsExecutive=false and AutoTalk=false.
func TestDrawerCreateDefaultsOff(t *testing.T) {
	d := newAgentDrawer()
	d.OpenCreate(nil, 0)

	msgs := concat([]tea.Msg{typeRunes("scout")}, tabsToSave())
	_, saved := driveDrawer(t, d, msgs...)
	if saved == nil {
		t.Fatal("expected a DrawerSaveMsg")
	}
	if saved.Agent.IsExecutive {
		t.Error("new agent must default to IsExecutive=false")
	}
	if saved.Agent.AutoTalk {
		t.Error("new agent must default to AutoTalk=false")
	}
}
