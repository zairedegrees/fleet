package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zairedegrees/fleet/internal/config"
)

// Messages emitted by the agents panel for the parent model to handle.
type EditAgentMsg struct{ Index int }
type NewAgentMsg struct{}
type DeleteAgentMsg struct{ Index int }

// agentsPanel is the right panel sub-model.
type agentsPanel struct {
	items      []agentItem
	cursor     int
	width      int
	compressed bool // true when drawer is open, show compact view
}

func newAgentsPanel() agentsPanel {
	return agentsPanel{}
}

// SetAgents replaces the agent list (called when preset selected).
func (p *agentsPanel) SetAgents(items []agentItem) {
	p.items = items
	p.cursor = 0
}

// AddAgent appends a new agent and enables it.
func (p *agentsPanel) AddAgent(agent config.AgentConfig) {
	p.items = append(p.items, agentItem{agent: agent, enabled: true})
}

// UpdateAgent updates the agent at index.
func (p *agentsPanel) UpdateAgent(index int, agent config.AgentConfig) {
	if index >= 0 && index < len(p.items) {
		p.items[index].agent = agent
	}
}

// RemoveAgent removes the agent at index.
func (p *agentsPanel) RemoveAgent(index int) {
	if index >= 0 && index < len(p.items) {
		p.items = append(p.items[:index], p.items[index+1:]...)
		if p.cursor >= len(p.items) && p.cursor > 0 {
			p.cursor--
		}
	}
}

// EnabledAgents returns only enabled agents.
func (p agentsPanel) EnabledAgents() []config.AgentConfig {
	var agents []config.AgentConfig
	for _, item := range p.items {
		if item.enabled {
			agents = append(agents, item.agent)
		}
	}
	return agents
}

// EnabledCount returns how many agents are enabled.
func (p agentsPanel) EnabledCount() int {
	count := 0
	for _, item := range p.items {
		if item.enabled {
			count++
		}
	}
	return count
}

// CurrentAgent returns the agent under the cursor, or nil.
func (p agentsPanel) CurrentAgent() *config.AgentConfig {
	if p.cursor >= 0 && p.cursor < len(p.items) {
		return &p.items[p.cursor].agent
	}
	return nil
}

func (p agentsPanel) Update(msg tea.Msg) (agentsPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p agentsPanel) handleKey(msg tea.KeyMsg) (agentsPanel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "j":
		if p.cursor < len(p.items)-1 {
			p.cursor++
		}
	case " ":
		if p.cursor >= 0 && p.cursor < len(p.items) {
			p.items[p.cursor].enabled = !p.items[p.cursor].enabled
		}
	case "a":
		// Toggle all
		allOn := true
		for _, item := range p.items {
			if !item.enabled {
				allOn = false
				break
			}
		}
		for i := range p.items {
			p.items[i].enabled = !allOn
		}
	case "e":
		if p.cursor >= 0 && p.cursor < len(p.items) {
			return p, func() tea.Msg { return EditAgentMsg{Index: p.cursor} }
		}
	case "n":
		return p, func() tea.Msg { return NewAgentMsg{} }
	case "d", "delete":
		if p.cursor >= 0 && p.cursor < len(p.items) {
			return p, func() tea.Msg { return DeleteAgentMsg{Index: p.cursor} }
		}
	}
	return p, nil
}

// View renders the agents panel. active=true highlights the border.
func (p agentsPanel) View(active bool) string {
	var sb strings.Builder

	borderColor := lipgloss.Color("238")
	if active {
		borderColor = lipgloss.Color("99")
	}

	enabledCount := p.EnabledCount()
	cols, rows := autoLayout(enabledCount)

	sb.WriteString(dimStyle.Render(fmt.Sprintf("AGENTS (%d) — %dx%d grid", enabledCount, cols, rows)) + "\n")

	if len(p.items) == 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  No agents — press n to create\n  one or select a preset.") + "\n")
	} else if p.compressed {
		// Compact view when drawer is open
		var names []string
		for _, item := range p.items {
			if item.enabled {
				names = append(names, item.agent.Name)
			}
		}
		sb.WriteString("  " + dimStyle.Render(strings.Join(names, "  ")) + "\n")
	} else {
		// Full list view
		for i, item := range p.items {
			cursor := "  "
			style := dimStyle
			if active && i == p.cursor {
				cursor = selectedStyle.Render("▸ ")
				style = selectedStyle
			}

			check := "[ ]"
			if item.enabled {
				check = selectedStyle.Render("[x]")
			}

			// Color dot
			colorDot := colorToAnsi(item.agent.Color)

			name := fmt.Sprintf("%-14s", item.agent.Name)
			role := dimStyle.Render(item.agent.Role)

			sb.WriteString(cursor + check + " " + style.Render(name) + " " + colorDot + " " + role + "\n")
		}
		sb.WriteString(dimStyle.Render("  + New agent (n)") + "\n")
	}

	panelStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		PaddingLeft(1).
		PaddingRight(1)

	if p.width > 0 {
		panelStyle = panelStyle.Width(p.width)
	}

	return panelStyle.Render(sb.String())
}

// colorToAnsi returns a colored dot for the agent color.
func colorToAnsi(color string) string {
	colorMap := map[string]string{
		"green":  "2",
		"orange": "208",
		"blue":   "4",
		"red":    "1",
		"purple": "5",
		"pink":   "13",
		"cyan":   "6",
		"yellow": "3",
	}
	code, ok := colorMap[color]
	if !ok {
		code = "7" // white default
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(code)).Render("●")
}
