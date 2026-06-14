package wizard

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zairedegrees/fleet/internal/config"
)

// DrawerSaveMsg is sent when the drawer saves an agent.
type DrawerSaveMsg struct {
	Agent config.AgentConfig
	Index int // -1 for new agent
}

// DrawerCancelMsg is sent when the drawer is cancelled.
type DrawerCancelMsg struct{}

type drawerField int

const (
	dfName drawerField = iota
	dfRole
	dfColor
	dfReportsTo
	dfAutoTalk
	dfExecutive
)

// autoTalkOpts are the auto-talk toggle options (index 1 = on).
var autoTalkOpts = []string{"off", "on"}

// executiveOpts are the executive toggle options (index 1 = on).
var executiveOpts = []string{"off", "on"}

// fieldKind classifies how a drawer field is edited and rendered.
type fieldKind int

const (
	kindText fieldKind = iota
	kindSelect
)

// fieldSpec describes one drawer field. drawerFields is the single ordered
// source of truth for navigation, the commit-on-last decision, and rendering —
// adding a field means adding a row here (plus its backing storage), not editing
// three separate hardcoded chains (nextField, the select-commit, and View).
type fieldSpec struct {
	id    drawerField
	kind  fieldKind
	label string // padded label text, rendered after the focus marker
}

var drawerFields = []fieldSpec{
	{dfName, kindText, "Name:       "},
	{dfRole, kindText, "Role:       "},
	{dfColor, kindSelect, "Color:      "},
	{dfReportsTo, kindSelect, "Reports to: "},
	{dfAutoTalk, kindSelect, "Auto-talk:  "},
	{dfExecutive, kindSelect, "Executive:  "},
}

func fieldIndex(id drawerField) int {
	for i, f := range drawerFields {
		if f.id == id {
			return i
		}
	}
	return 0
}

// isLastField reports whether id is the final field — entering past it saves.
func isLastField(id drawerField) bool { return fieldIndex(id) == len(drawerFields)-1 }

// agentDrawer is the bottom drawer sub-model for edit/create.
type agentDrawer struct {
	nameInput    textinput.Model
	roleInput    textinput.Model
	colorIdx     int
	reportsIdx   int
	reportOpts   []string // "(none)" + agent names
	autoTalkIdx  int      // index into autoTalkOpts
	executiveIdx int      // index into executiveOpts

	// base is the agent being edited (zero for create): save() starts from it
	// so any future AgentConfig field the drawer doesn't manage survives an
	// edit instead of being dropped. Today every field IS drawer-managed, so
	// this capture is unobservable behavior — an equivalent mutant that no
	// test can pin until config grows an unmanaged field.
	base config.AgentConfig

	field     drawerField
	mode      drawerMode // drawerEdit or drawerCreate
	editIndex int        // index of agent being edited, -1 for new
	title     string
}

func newAgentDrawer() agentDrawer {
	ni := textinput.New()
	ni.Placeholder = "agent-name"
	ni.CharLimit = 30
	ni.Width = 25

	ri := textinput.New()
	ri.Placeholder = "Agent role description"
	ri.CharLimit = 60
	ri.Width = 25

	return agentDrawer{
		nameInput: ni,
		roleInput: ri,
	}
}

// OpenEdit opens the drawer to edit an existing agent.
func (d *agentDrawer) OpenEdit(index int, agent config.AgentConfig, agentNames []string) {
	d.mode = drawerEdit
	d.editIndex = index
	d.title = "Edit: " + agent.Name
	d.field = dfName
	d.base = agent

	d.nameInput.SetValue(agent.Name)
	d.nameInput.Focus()
	d.roleInput.SetValue(agent.Role)

	// Find color index
	d.colorIdx = 0
	for i, c := range agentColors {
		if c == agent.Color {
			d.colorIdx = i
			break
		}
	}

	// Build reports-to options
	d.reportOpts = []string{"(none)"}
	d.reportsIdx = 0
	for _, name := range agentNames {
		if name != agent.Name {
			d.reportOpts = append(d.reportOpts, name)
		}
	}
	for i, opt := range d.reportOpts {
		if opt == agent.ReportsTo {
			d.reportsIdx = i
			break
		}
	}

	d.autoTalkIdx = 0
	if agent.AutoTalk {
		d.autoTalkIdx = 1
	}

	d.executiveIdx = 0
	if agent.IsExecutive {
		d.executiveIdx = 1
	}
}

// OpenCreate opens the drawer for a new agent.
func (d *agentDrawer) OpenCreate(agentNames []string, nextColorIdx int) {
	d.mode = drawerCreate
	d.editIndex = -1
	d.title = "New Agent"
	d.field = dfName
	d.base = config.AgentConfig{}

	d.nameInput.SetValue("")
	d.nameInput.Focus()
	d.roleInput.SetValue("")
	d.colorIdx = nextColorIdx % len(agentColors)

	d.reportOpts = []string{"(none)"}
	d.reportsIdx = 0
	for _, name := range agentNames {
		d.reportOpts = append(d.reportOpts, name)
	}
	d.autoTalkIdx = 0
	d.executiveIdx = 0
}

func (d agentDrawer) Update(msg tea.Msg) (agentDrawer, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// handleKey takes a pointer receiver: helpers that mutate fields
		// through pointers must write into this same d, not a copy.
		cmd := d.handleKey(msg)
		return d, cmd
	}

	// Forward to active text input
	var cmd tea.Cmd
	switch d.field {
	case dfName:
		d.nameInput, cmd = d.nameInput.Update(msg)
	case dfRole:
		d.roleInput, cmd = d.roleInput.Update(msg)
	}
	return d, cmd
}

// textInput returns the text input backing a kindText field, or nil.
func (d *agentDrawer) textInput(id drawerField) *textinput.Model {
	switch id {
	case dfName:
		return &d.nameInput
	case dfRole:
		return &d.roleInput
	}
	return nil
}

// selectState returns the options and the selected index for a kindSelect field
// (the read path, used by View).
func (d *agentDrawer) selectState(id drawerField) ([]string, int) {
	switch id {
	case dfColor:
		return agentColors, d.colorIdx
	case dfReportsTo:
		return d.reportOpts, d.reportsIdx
	case dfAutoTalk:
		return autoTalkOpts, d.autoTalkIdx
	case dfExecutive:
		return executiveOpts, d.executiveIdx
	}
	return nil, 0
}

// selectPtr returns a pointer to the selected-index and the option count for a
// kindSelect field (the write path, used by key handling).
func (d *agentDrawer) selectPtr(id drawerField) (*int, int) {
	switch id {
	case dfColor:
		return &d.colorIdx, len(agentColors)
	case dfReportsTo:
		return &d.reportsIdx, len(d.reportOpts)
	case dfAutoTalk:
		return &d.autoTalkIdx, len(autoTalkOpts)
	case dfExecutive:
		return &d.executiveIdx, len(executiveOpts)
	}
	return nil, 0
}

func (d *agentDrawer) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		return func() tea.Msg { return DrawerCancelMsg{} }
	case "tab":
		return d.nextField()
	}

	switch drawerFields[fieldIndex(d.field)].kind {
	case kindText:
		return d.handleTextField(msg, d.textInput(d.field))
	case kindSelect:
		idx, count := d.selectPtr(d.field)
		return d.handleSelectField(msg, idx, count)
	}
	return nil
}

func (d *agentDrawer) handleTextField(msg tea.KeyMsg, input *textinput.Model) tea.Cmd {
	switch msg.String() {
	case "enter":
		return d.nextField()
	}
	var cmd tea.Cmd
	*input, cmd = input.Update(msg)
	return cmd
}

func (d *agentDrawer) handleSelectField(msg tea.KeyMsg, idx *int, count int) tea.Cmd {
	switch msg.String() {
	case "left", "h":
		if *idx > 0 {
			*idx--
		}
	case "right", "l":
		if *idx < count-1 {
			*idx++
		}
	case "up", "k":
		if *idx > 0 {
			*idx--
		}
	case "down", "j":
		if *idx < count-1 {
			*idx++
		}
	case "enter":
		if isLastField(d.field) {
			return d.save()
		}
		return d.nextField()
	}
	return nil
}

// nextField advances to the next field in drawerFields, saving when it steps
// past the last one. Leaving a text field blurs it; entering one focuses it.
// The name guard keeps an empty name from advancing, mirroring the create flow.
func (d *agentDrawer) nextField() tea.Cmd {
	if d.field == dfName && strings.TrimSpace(d.nameInput.Value()) == "" {
		return nil
	}
	i := fieldIndex(d.field)
	if i == len(drawerFields)-1 {
		return d.save()
	}
	if in := d.textInput(d.field); in != nil {
		in.Blur()
	}
	d.field = drawerFields[i+1].id
	if in := d.textInput(d.field); in != nil {
		in.Focus()
		return textinput.Blink
	}
	return nil
}

func (d *agentDrawer) save() tea.Cmd {
	name := normalizeName(d.nameInput.Value())
	if name == "" {
		return nil
	}

	reportsTo := ""
	if d.reportsIdx > 0 {
		reportsTo = d.reportOpts[d.reportsIdx]
	}

	agent := d.base
	agent.Name = name
	agent.Color = agentColors[d.colorIdx]
	agent.Role = strings.TrimSpace(d.roleInput.Value())
	agent.ReportsTo = reportsTo
	agent.AutoTalk = d.autoTalkIdx == 1
	agent.IsExecutive = d.executiveIdx == 1

	index := d.editIndex
	return func() tea.Msg {
		return DrawerSaveMsg{Agent: agent, Index: index}
	}
}

func (d agentDrawer) View() string {
	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("99"))

	sb.WriteString(headerStyle.Render(d.title) + "\n")

	// Fields, driven by the drawerFields table. A text field shows its live
	// input when focused, its value otherwise; a select field lists its options
	// with the chosen one bracketed. Adding a field is one row in drawerFields.
	for _, spec := range drawerFields {
		focused := d.field == spec.id
		label := dimStyle.Render("  " + spec.label)
		if focused {
			label = selectedStyle.Render("▸ " + spec.label)
		}

		switch spec.kind {
		case kindText:
			in := d.textInput(spec.id)
			if focused {
				sb.WriteString(label + in.View() + "\n")
			} else {
				sb.WriteString(label + in.Value() + "\n")
			}
		case kindSelect:
			sb.WriteString(label)
			opts, sel := d.selectState(spec.id)
			for i, opt := range opts {
				if i == sel {
					sb.WriteString(selectedStyle.Render("["+opt+"]") + " ")
				} else {
					sb.WriteString(dimStyle.Render(opt) + " ")
				}
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n" + dimStyle.Render("  tab=next  enter=save  esc=cancel"))

	return sb.String()
}
