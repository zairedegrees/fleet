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
	// so fields the drawer doesn't manage survive an edit instead of being dropped.
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

func (d *agentDrawer) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		return func() tea.Msg { return DrawerCancelMsg{} }
	case "tab":
		return d.nextField()
	}

	switch d.field {
	case dfName:
		return d.handleTextField(msg, &d.nameInput)
	case dfRole:
		return d.handleTextField(msg, &d.roleInput)
	case dfColor:
		return d.handleSelectField(msg, &d.colorIdx, len(agentColors))
	case dfReportsTo:
		return d.handleSelectField(msg, &d.reportsIdx, len(d.reportOpts))
	case dfAutoTalk:
		return d.handleSelectField(msg, &d.autoTalkIdx, len(autoTalkOpts))
	case dfExecutive:
		return d.handleSelectField(msg, &d.executiveIdx, len(executiveOpts))
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
		if d.field == dfExecutive {
			return d.save()
		}
		return d.nextField()
	}
	return nil
}

func (d *agentDrawer) nextField() tea.Cmd {
	switch d.field {
	case dfName:
		if strings.TrimSpace(d.nameInput.Value()) == "" {
			return nil
		}
		d.field = dfRole
		d.nameInput.Blur()
		d.roleInput.Focus()
		return textinput.Blink
	case dfRole:
		d.field = dfColor
		d.roleInput.Blur()
		return nil
	case dfColor:
		d.field = dfReportsTo
		return nil
	case dfReportsTo:
		d.field = dfAutoTalk
		return nil
	case dfAutoTalk:
		d.field = dfExecutive
		return nil
	case dfExecutive:
		return d.save()
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

	// Name field
	label := dimStyle.Render("  Name:       ")
	if d.field == dfName {
		label = selectedStyle.Render("▸ Name:       ")
		sb.WriteString(label + d.nameInput.View() + "\n")
	} else {
		sb.WriteString(label + d.nameInput.Value() + "\n")
	}

	// Role field
	label = dimStyle.Render("  Role:       ")
	if d.field == dfRole {
		label = selectedStyle.Render("▸ Role:       ")
		sb.WriteString(label + d.roleInput.View() + "\n")
	} else {
		sb.WriteString(label + d.roleInput.Value() + "\n")
	}

	// Color field
	label = dimStyle.Render("  Color:      ")
	if d.field == dfColor {
		label = selectedStyle.Render("▸ Color:      ")
	}
	sb.WriteString(label)
	for i, c := range agentColors {
		dot := colorToAnsi(c)
		if i == d.colorIdx {
			sb.WriteString(selectedStyle.Render("["+c+"]") + " ")
		} else {
			sb.WriteString(dimStyle.Render(c) + " ")
		}
		_ = dot
	}
	sb.WriteString("\n")

	// Reports-to field
	label = dimStyle.Render("  Reports to: ")
	if d.field == dfReportsTo {
		label = selectedStyle.Render("▸ Reports to: ")
	}
	sb.WriteString(label)
	for i, opt := range d.reportOpts {
		if i == d.reportsIdx {
			sb.WriteString(selectedStyle.Render("["+opt+"]") + " ")
		} else {
			sb.WriteString(dimStyle.Render(opt) + " ")
		}
	}
	sb.WriteString("\n")

	// Auto-talk field
	label = dimStyle.Render("  Auto-talk:  ")
	if d.field == dfAutoTalk {
		label = selectedStyle.Render("▸ Auto-talk:  ")
	}
	sb.WriteString(label)
	for i, opt := range autoTalkOpts {
		if i == d.autoTalkIdx {
			sb.WriteString(selectedStyle.Render("["+opt+"]") + " ")
		} else {
			sb.WriteString(dimStyle.Render(opt) + " ")
		}
	}
	sb.WriteString("\n")

	// Executive field
	label = dimStyle.Render("  Executive:  ")
	if d.field == dfExecutive {
		label = selectedStyle.Render("▸ Executive:  ")
	}
	sb.WriteString(label)
	for i, opt := range executiveOpts {
		if i == d.executiveIdx {
			sb.WriteString(selectedStyle.Render("["+opt+"]") + " ")
		} else {
			sb.WriteString(dimStyle.Render(opt) + " ")
		}
	}
	sb.WriteString("\n")

	sb.WriteString("\n" + dimStyle.Render("  tab=next  enter=save  esc=cancel"))

	return sb.String()
}
