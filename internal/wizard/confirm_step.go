package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nazaire/fleet/internal/config"
)

type confirmChoice int

const (
	choiceLaunch confirmChoice = iota
	choiceSaveAndLaunch
	choiceCancel
)

var confirmChoices = []string{
	"Yes, launch",
	"Save config & launch",
	"Cancel",
}

type confirmModel struct {
	project  string
	agents   []config.AgentConfig
	cols     int
	rows     int
	cursor   int
	choice   confirmChoice
	quitting bool
	chosen   bool
}

func runConfirmStep(project string, agents []config.AgentConfig) (*WizardResult, error) {
	cols, rows := autoLayout(len(agents))

	m := confirmModel{
		project: project,
		agents:  agents,
		cols:    cols,
		rows:    rows,
	}

	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("confirm step: %w", err)
	}

	fm := final.(confirmModel)
	if fm.quitting || !fm.chosen {
		return nil, fmt.Errorf("cancelled")
	}

	if fm.choice == choiceCancel {
		return nil, fmt.Errorf("cancelled")
	}

	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{
			Name: project,
		},
		Agents: agents,
	}

	return &WizardResult{
		Config: cfg,
		Save:   fm.choice == choiceSaveAndLaunch,
	}, nil
}

func (m confirmModel) Init() tea.Cmd {
	return nil
}

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(confirmChoices)-1 {
				m.cursor++
			}

		case "enter":
			m.choice = confirmChoice(m.cursor)
			m.chosen = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Confirm fleet") + "\n\n")

	// Summary
	sb.WriteString(fmt.Sprintf("  Project:  %s\n", selectedStyle.Render(m.project)))
	sb.WriteString(fmt.Sprintf("  Agents:   %s\n", selectedStyle.Render(fmt.Sprintf("%d", len(m.agents)))))
	sb.WriteString(fmt.Sprintf("  Layout:   %s\n", selectedStyle.Render(fmt.Sprintf("%dx%d grid", m.cols, m.rows))))
	sb.WriteString("\n")

	// Agent table
	sb.WriteString("  " + dimStyle.Render(fmt.Sprintf("%-16s %-10s %s", "NAME", "COLOR", "ROLE")) + "\n")
	sb.WriteString("  " + dimStyle.Render(strings.Repeat("-", 50)) + "\n")
	for _, a := range m.agents {
		color := a.Color
		if color == "" {
			color = "-"
		}
		role := a.Role
		if role == "" {
			role = "-"
		}
		sb.WriteString(fmt.Sprintf("  %-16s %-10s %s\n", a.Name, color, role))
	}
	sb.WriteString("\n")

	// Choices
	for i, label := range confirmChoices {
		cursor := "  "
		style := dimStyle
		if m.cursor == i {
			cursor = selectedStyle.Render("> ")
			style = selectedStyle
		}
		sb.WriteString(cursor + style.Render(label) + "\n")
	}

	sb.WriteString("\n" + dimStyle.Render("  j/k = move  enter = select  q = quit"))
	return sb.String()
}
