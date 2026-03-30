package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nazaire/fleet/internal/config"
	"github.com/nazaire/fleet/internal/relay"
)

var agentColors = []string{
	"green", "orange", "blue", "red", "purple", "pink", "cyan", "yellow",
}

// agentItem represents a selectable agent in the list.
type agentItem struct {
	agent    config.AgentConfig
	enabled  bool
	isCreate bool // "Create new agent..." sentinel
}

type createFormField int

const (
	fieldName createFormField = iota
	fieldRole
	fieldColor
	fieldReportsTo
)

// agentsModel is the bubbletea model for agent selection + creation.
type agentsModel struct {
	items    []agentItem
	cursor   int
	quitting bool
	err      error

	// Inline create-agent form
	creating     bool
	formField    createFormField
	formName     string
	formRole     string
	formColorIdx int
	formReportTo int    // index into reportToOptions
	reportOpts   []string // agent names + "(none)"
}

func runAgentsStep(relayClient *relay.Client, project string, isNew bool) ([]config.AgentConfig, error) {
	items := gatherAgents(relayClient, project, isNew)

	m := agentsModel{
		items: items,
	}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("agents step: %w", err)
	}

	fm := final.(agentsModel)
	if fm.err != nil {
		return nil, fm.err
	}
	if fm.quitting {
		return nil, fmt.Errorf("cancelled")
	}

	var agents []config.AgentConfig
	for _, item := range fm.items {
		if item.enabled && !item.isCreate {
			agents = append(agents, item.agent)
		}
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents selected")
	}
	return agents, nil
}

func gatherAgents(relayClient *relay.Client, project string, isNew bool) []agentItem {
	var items []agentItem

	if !isNew && relayClient != nil {
		if agents, err := relayClient.ListAgents(project); err == nil {
			for i, a := range agents {
				color := a.Color
				if color == "" {
					color = agentColors[i%len(agentColors)]
				}
				items = append(items, agentItem{
					agent: config.AgentConfig{
						Name:        a.Name,
						Color:       color,
						Role:        a.Role,
						ReportsTo:   a.ReportsTo,
						IsExecutive: a.IsExecutive,
					},
					enabled: true,
				})
			}
		}
	}

	// Append the "Create new..." sentinel
	items = append(items, agentItem{isCreate: true})
	return items
}

func (m agentsModel) Init() tea.Cmd {
	return nil
}

func (m agentsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.creating {
			return m.updateForm(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m agentsModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}

	case " ":
		item := &m.items[m.cursor]
		if item.isCreate {
			m.startCreate()
		} else {
			item.enabled = !item.enabled
		}

	case "a":
		// Toggle all non-sentinel items
		allOn := true
		for _, item := range m.items {
			if !item.isCreate && !item.enabled {
				allOn = false
				break
			}
		}
		for i := range m.items {
			if !m.items[i].isCreate {
				m.items[i].enabled = !allOn
			}
		}

	case "enter":
		cur := m.items[m.cursor]
		if cur.isCreate {
			m.startCreate()
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m *agentsModel) startCreate() {
	m.creating = true
	m.formField = fieldName
	m.formName = ""
	m.formRole = ""
	m.formColorIdx = 0
	m.formReportTo = 0

	// Build reports-to options from existing (non-sentinel) agents
	m.reportOpts = []string{"(none)"}
	for _, item := range m.items {
		if !item.isCreate && item.agent.Name != "" {
			m.reportOpts = append(m.reportOpts, item.agent.Name)
		}
	}
}

func (m agentsModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "esc":
		m.creating = false
		return m, nil
	}

	switch m.formField {
	case fieldName:
		return m.updateFormText(msg, &m.formName, fieldRole)
	case fieldRole:
		return m.updateFormText(msg, &m.formRole, fieldColor)
	case fieldColor:
		return m.updateFormSelect(msg, &m.formColorIdx, len(agentColors), fieldReportsTo)
	case fieldReportsTo:
		return m.updateFormSelect(msg, &m.formReportTo, len(m.reportOpts), -1)
	}
	return m, nil
}

// updateFormText handles text input fields. nextField is advanced on enter.
func (m agentsModel) updateFormText(msg tea.KeyMsg, target *string, nextField createFormField) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if strings.TrimSpace(*target) == "" {
			return m, nil
		}
		m.formField = nextField
	case "backspace":
		if len(*target) > 0 {
			*target = (*target)[:len(*target)-1]
		}
	default:
		if len(msg.String()) == 1 {
			*target += msg.String()
		}
	}
	return m, nil
}

// updateFormSelect handles list selection fields. nextField=-1 means finalize.
func (m agentsModel) updateFormSelect(msg tea.KeyMsg, idx *int, count int, nextField createFormField) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if *idx > 0 {
			*idx--
		}
	case "down", "j":
		if *idx < count-1 {
			*idx++
		}
	case "enter":
		if nextField < 0 {
			// Finalize: create the agent and add it to the list
			m.finalizeCreate()
			return m, nil
		}
		m.formField = nextField
	}
	return m, nil
}

func (m *agentsModel) finalizeCreate() {
	reportsTo := ""
	if m.formReportTo > 0 {
		reportsTo = m.reportOpts[m.formReportTo]
	}

	newAgent := agentItem{
		agent: config.AgentConfig{
			Name:      strings.TrimSpace(m.formName),
			Color:     agentColors[m.formColorIdx],
			Role:      strings.TrimSpace(m.formRole),
			ReportsTo: reportsTo,
		},
		enabled: true,
	}

	// Insert before the "Create new..." sentinel (last item)
	sentinel := m.items[len(m.items)-1]
	m.items = append(m.items[:len(m.items)-1], newAgent, sentinel)
	m.cursor = len(m.items) - 2 // point at newly created agent
	m.creating = false
}

func (m agentsModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Select agents") + "\n\n")

	if m.creating {
		sb.WriteString(m.viewForm())
		return sb.String()
	}

	for i, item := range m.items {
		cursor := "  "
		if m.cursor == i {
			cursor = selectedStyle.Render("> ")
		}

		if item.isCreate {
			style := dimStyle
			if m.cursor == i {
				style = selectedStyle
			}
			sb.WriteString(cursor + style.Render("+ Create new agent...") + "\n")
			continue
		}

		check := "[ ]"
		if item.enabled {
			check = selectedStyle.Render("[x]")
		}

		name := item.agent.Name
		style := dimStyle
		if m.cursor == i {
			style = selectedStyle
		}

		role := ""
		if item.agent.Role != "" {
			role = dimStyle.Render(" (" + item.agent.Role + ")")
		}

		sb.WriteString(cursor + check + " " + style.Render(name) + role + "\n")
	}

	sb.WriteString("\n" + dimStyle.Render("  space = toggle  a = toggle all  enter = confirm  q = quit"))
	return sb.String()
}

func (m agentsModel) viewForm() string {
	var sb strings.Builder

	sb.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("New Agent") + "\n\n")

	// Name
	label := dimStyle.Render("  Name:  ")
	if m.formField == fieldName {
		label = selectedStyle.Render("  Name:  ")
		sb.WriteString(label + selectedStyle.Render(m.formName))
		sb.WriteString(lipgloss.NewStyle().Blink(true).Render("_"))
	} else {
		sb.WriteString(label + m.formName)
	}
	sb.WriteString("\n")

	// Role
	label = dimStyle.Render("  Role:  ")
	if m.formField == fieldRole {
		label = selectedStyle.Render("  Role:  ")
		sb.WriteString(label + selectedStyle.Render(m.formRole))
		sb.WriteString(lipgloss.NewStyle().Blink(true).Render("_"))
	} else {
		sb.WriteString(label + m.formRole)
	}
	sb.WriteString("\n")

	// Color
	label = dimStyle.Render("  Color: ")
	if m.formField == fieldColor {
		label = selectedStyle.Render("  Color: ")
	}
	sb.WriteString(label)
	if m.formField == fieldColor {
		for i, c := range agentColors {
			if i == m.formColorIdx {
				sb.WriteString(selectedStyle.Render("[" + c + "] "))
			} else {
				sb.WriteString(dimStyle.Render(" " + c + "  "))
			}
		}
	} else if m.formField > fieldColor {
		sb.WriteString(agentColors[m.formColorIdx])
	}
	sb.WriteString("\n")

	// Reports to
	label = dimStyle.Render("  Reports to: ")
	if m.formField == fieldReportsTo {
		label = selectedStyle.Render("  Reports to: ")
	}
	sb.WriteString(label)
	if m.formField == fieldReportsTo {
		for i, opt := range m.reportOpts {
			if i == m.formReportTo {
				sb.WriteString(selectedStyle.Render("[" + opt + "] "))
			} else {
				sb.WriteString(dimStyle.Render(" " + opt + "  "))
			}
		}
	} else if m.formField > fieldReportsTo {
		sb.WriteString(m.reportOpts[m.formReportTo])
	}
	sb.WriteString("\n")

	sb.WriteString("\n" + dimStyle.Render("  enter = next  esc = cancel"))
	return sb.String()
}
