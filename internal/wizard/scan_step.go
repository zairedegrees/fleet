package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nazaire/fleet/internal/config"
	"github.com/nazaire/fleet/internal/scanner"
)

type scanModel struct {
	scan        *scanner.ScanResult
	suggestions []scanner.AgentSuggestion
	quitting    bool
	done        bool
}

// runScanStep scans the cwd and returns suggested agents.
// Returns nil suggestions if scan finds nothing notable (still works, just no pre-fill).
func runScanStep(cwd string) ([]config.AgentConfig, error) {
	result, err := scanner.Scan(cwd)
	if err != nil {
		// Non-fatal: return empty suggestions, wizard continues
		return nil, nil
	}

	suggestions := scanner.SuggestAgents(result)

	// If only dev agent suggested (minimal project), skip the scan display
	if len(suggestions) <= 1 {
		var agents []config.AgentConfig
		for _, s := range suggestions {
			agents = append(agents, s.Agent)
		}
		return agents, nil
	}

	m := scanModel{
		scan:        result,
		suggestions: suggestions,
	}

	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("scan step: %w", err)
	}

	fm := final.(scanModel)
	if fm.quitting {
		return nil, fmt.Errorf("cancelled")
	}

	var agents []config.AgentConfig
	for _, s := range suggestions {
		agents = append(agents, s.Agent)
	}
	return agents, nil
}

func (m scanModel) Init() tea.Cmd {
	return nil
}

func (m scanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m scanModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Project scan") + "\n\n")

	// Detected stack
	sb.WriteString("  Detected:\n")
	if len(m.scan.Languages) > 0 {
		sb.WriteString(fmt.Sprintf("    Languages:  %s\n", selectedStyle.Render(strings.Join(m.scan.Languages, ", "))))
	}
	if len(m.scan.Frameworks) > 0 {
		sb.WriteString(fmt.Sprintf("    Frameworks: %s\n", selectedStyle.Render(strings.Join(m.scan.Frameworks, ", "))))
	}
	if len(m.scan.Structure) > 0 {
		sb.WriteString(fmt.Sprintf("    Structure:  %s\n", dimStyle.Render(strings.Join(m.scan.Structure, ", "))))
	}
	sb.WriteString("\n")

	// Suggested agents
	sb.WriteString("  Suggested agents:\n")
	for _, s := range m.suggestions {
		color := s.Agent.Color
		name := s.Agent.Name
		role := s.Agent.Role
		sb.WriteString(fmt.Sprintf("    %s (%s) — %s\n",
			selectedStyle.Render(name),
			dimStyle.Render(color),
			dimStyle.Render(role)))
	}
	sb.WriteString("\n")

	sb.WriteString(dimStyle.Render("  Press Enter to continue with these agents, or edit in next step."))
	sb.WriteString("\n" + dimStyle.Render("  q = quit"))

	return sb.String()
}
