package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nazaire/fleet/internal/config"
	"github.com/nazaire/fleet/internal/relay"
)

// projectResult is the outcome of the project selection step.
type projectResult struct {
	name  string
	isNew bool
}

// projectModel is the bubbletea model for the project selection step.
type projectModel struct {
	projects []string // combined relay + saved config names
	cursor   int
	hasNew   bool // whether the "Create new..." option is shown
	typing   bool // whether the user is typing a new project name
	input    string
	result   *projectResult
	err      error
	quitting bool
}

func runProjectStep(relayClient *relay.Client) (string, bool, error) {
	projects := gatherProjects(relayClient)

	m := projectModel{
		projects: projects,
		hasNew:   true,
	}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", false, fmt.Errorf("project step: %w", err)
	}

	fm := final.(projectModel)
	if fm.err != nil {
		return "", false, fm.err
	}
	if fm.quitting || fm.result == nil {
		return "", false, fmt.Errorf("cancelled")
	}
	return fm.result.name, fm.result.isNew, nil
}

// gatherProjects discovers projects from saved configs and by probing the relay.
func gatherProjects(relayClient *relay.Client) []string {
	seen := make(map[string]bool)
	var projects []string

	// 1. Scan saved configs in ~/.fleet/configs/
	configDir := filepath.Join(config.FleetDir(), "configs")
	entries, err := os.ReadDir(configDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".toml")
			if !seen[name] {
				seen[name] = true
				projects = append(projects, name)
			}
		}
	}

	// 2. Scan known project names from ~/.fleet/projects (one per line)
	projectsFile := filepath.Join(config.FleetDir(), "projects")
	data, err := os.ReadFile(projectsFile)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			name := strings.TrimSpace(line)
			if name != "" && !seen[name] {
				seen[name] = true
				projects = append(projects, name)
			}
		}
	}

	// 3. Probe relay for each candidate — verify it has agents
	if relayClient != nil {
		var verified []string
		for _, p := range projects {
			agents, err := relayClient.ListAgents(p)
			if err == nil && len(agents) > 0 {
				verified = append(verified, p)
			}
		}
		// If we found verified projects, prefer those. Otherwise keep all candidates.
		if len(verified) > 0 {
			return verified
		}
	}

	return projects
}

func (m projectModel) Init() tea.Cmd {
	return nil
}

func (m projectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.typing {
			return m.updateTyping(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m projectModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalItems := len(m.projects)
	if m.hasNew {
		totalItems++
	}

	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < totalItems-1 {
			m.cursor++
		}

	case "enter":
		if m.hasNew && m.cursor == len(m.projects) {
			// "Create new..." selected
			m.typing = true
			m.input = ""
		} else if m.cursor < len(m.projects) {
			m.result = &projectResult{
				name:  m.projects[m.cursor],
				isNew: false,
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m projectModel) updateTyping(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.typing = false
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.input)
		if name == "" {
			return m, nil
		}
		m.result = &projectResult{
			name:  name,
			isNew: true,
		}
		return m, tea.Quit
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.input += msg.String()
		}
	}
	return m, nil
}

func (m projectModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Select project") + "\n\n")

	if m.typing {
		sb.WriteString("  Project name: ")
		sb.WriteString(selectedStyle.Render(m.input))
		sb.WriteString(lipgloss.NewStyle().Blink(true).Render("_"))
		sb.WriteString("\n\n")
		sb.WriteString(dimStyle.Render("  enter = confirm  esc = back"))
		return sb.String()
	}

	for i, p := range m.projects {
		cursor := "  "
		style := dimStyle
		if m.cursor == i {
			cursor = selectedStyle.Render("> ")
			style = selectedStyle
		}
		sb.WriteString(cursor + style.Render(p) + "\n")
	}

	if m.hasNew {
		idx := len(m.projects)
		cursor := "  "
		style := dimStyle
		if m.cursor == idx {
			cursor = selectedStyle.Render("> ")
			style = selectedStyle
		}
		sb.WriteString(cursor + style.Render("+ Create new project...") + "\n")
	}

	sb.WriteString("\n" + dimStyle.Render("  j/k = move  enter = select  q = quit"))

	return sb.String()
}
