package wizard

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nazaire/fleet/internal/config"
	"github.com/nazaire/fleet/internal/relay"
	"github.com/nazaire/fleet/internal/scanner"
)

type wizardModel struct {
	// Panels
	project projectPanel
	agents  agentsPanel
	drawer  agentDrawer

	// State
	activePanel panel
	drawerOpen  bool
	relayClient *relay.Client
	width       int
	height      int

	// Result
	quitting  bool
	launching bool
	saving    bool
}

func newWizardModel(relayClient *relay.Client) wizardModel {
	return wizardModel{
		project:     newProjectPanel(),
		agents:      newAgentsPanel(),
		drawer:      newAgentDrawer(),
		activePanel: panelLeft,
		relayClient: relayClient,
	}
}

func (m wizardModel) Init() tea.Cmd {
	if m.project.showExisting {
		return nil // project list is shown, no text input to focus
	}
	return m.project.nameInput.Focus()
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Split: left panel ~35%, right panel ~65%
		leftWidth := m.width*35/100 - 2
		rightWidth := m.width - leftWidth - 4
		if leftWidth < 20 {
			leftWidth = 20
		}
		m.project.width = leftWidth
		m.agents.width = rightWidth
		return m, nil

	case ProjectLoadedMsg:
		// Existing project loaded — populate everything from config
		cfg := msg.Config
		m.project.nameInput.SetValue(cfg.Project.Name)
		m.project.pathInput.SetValue(cfg.Project.Cwd)
		m.project.ready = true
		m.project.showExisting = false
		m.project.focus = focusPresets
		// Load agents from the saved config
		var items []agentItem
		for _, a := range cfg.Agents {
			items = append(items, agentItem{agent: a, enabled: true})
		}
		m.agents.SetAgents(items)
		m.activePanel = panelRight
		return m, nil

	case PresetSelectedMsg:
		// Preset chosen in left panel -> populate right panel
		preset := msg.Preset
		if preset.Name == "Custom" {
			// Run scanner on project path
			path := m.project.ProjectPath()
			if path != "" {
				if result, err := scanner.Scan(path); err == nil {
					suggestions := scanner.SuggestAgents(result)
					var items []agentItem
					for _, s := range suggestions {
						items = append(items, agentItem{agent: s.Agent, enabled: true})
					}
					m.agents.SetAgents(items)
				}
			}
			if len(m.agents.items) == 0 {
				m.agents.SetAgents(nil)
			}
		} else {
			m.agents.SetAgents(PresetAgentItems(preset))
		}
		m.activePanel = panelRight
		return m, nil

	case EditAgentMsg:
		// Open drawer in edit mode
		var names []string
		for _, item := range m.agents.items {
			names = append(names, item.agent.Name)
		}
		m.drawer.OpenEdit(msg.Index, m.agents.items[msg.Index].agent, names)
		m.drawerOpen = true
		m.agents.compressed = true
		return m, nil

	case NewAgentMsg:
		var names []string
		for _, item := range m.agents.items {
			names = append(names, item.agent.Name)
		}
		m.drawer.OpenCreate(names, len(m.agents.items))
		m.drawerOpen = true
		m.agents.compressed = true
		return m, nil

	case DeleteAgentMsg:
		m.agents.RemoveAgent(msg.Index)
		return m, nil

	case DrawerSaveMsg:
		if msg.Index >= 0 {
			m.agents.UpdateAgent(msg.Index, msg.Agent)
		} else {
			m.agents.AddAgent(msg.Agent)
		}
		m.drawerOpen = false
		m.agents.compressed = false
		return m, nil

	case DrawerCancelMsg:
		m.drawerOpen = false
		m.agents.compressed = false
		return m, nil
	}

	// Key routing
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// ctrl+c always quits
		if keyMsg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		// When in text input mode, delegate everything to the active component
		isTextInput := (m.activePanel == panelLeft && (m.project.focus == focusName || m.project.focus == focusPath)) || m.drawerOpen

		if !isTextInput {
			switch keyMsg.String() {
			case "q", "esc":
				m.quitting = true
				return m, tea.Quit
			case "tab":
				if m.project.IsReady() {
					if m.activePanel == panelLeft {
						m.activePanel = panelRight
					} else {
						m.activePanel = panelLeft
					}
					return m, nil
				}
			case "enter":
				// Only launch if right panel is active and project is ready
				if m.activePanel == panelRight && m.project.IsReady() {
					if m.agents.EnabledCount() > 0 {
						m.launching = true
						return m, tea.Quit
					}
				}
			case "s":
				// Save + launch
				if m.activePanel == panelRight && m.project.IsReady() {
					if m.agents.EnabledCount() > 0 {
						m.launching = true
						m.saving = true
						return m, tea.Quit
					}
				}
			}
		}
	}

	// Delegate to active component
	var cmd tea.Cmd
	if m.drawerOpen {
		m.drawer, cmd = m.drawer.Update(msg)
	} else if m.activePanel == panelLeft {
		m.project, cmd = m.project.Update(msg)
	} else {
		m.agents, cmd = m.agents.Update(msg)
	}
	return m, cmd
}

func (m wizardModel) View() string {
	var sb strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render("⚡ Fleet Wizard")
	sb.WriteString(title + "\n\n")

	// Split panels
	leftView := m.project.View(m.activePanel == panelLeft && !m.drawerOpen)

	var rightView string
	if m.drawerOpen {
		// Compressed agent list + drawer
		agentCompressed := m.agents.View(false)
		drawerView := m.drawer.View()
		rightView = agentCompressed + "\n" + drawerView
	} else {
		rightView = m.agents.View(m.activePanel == panelRight)
	}

	// Join horizontally
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)
	sb.WriteString(content)
	sb.WriteString("\n")

	// Help bar
	var help string
	if m.drawerOpen {
		help = "tab=field  j/k=select  enter=save  esc=cancel"
	} else if m.project.focus == focusProjectList {
		help = "j/k=move  enter=select  q=quit"
	} else if m.activePanel == panelLeft && (m.project.focus == focusName || m.project.focus == focusPath) {
		help = "type to enter  enter=confirm  esc=back  ctrl+c=quit"
	} else if m.activePanel == panelLeft {
		help = "j/k=move  enter=select preset  tab=agents panel  q=quit"
	} else {
		help = "j/k=move  space=toggle  e=edit  n=new  d=del  a=all  enter=launch  s=save+launch  tab=presets  q=quit"
	}
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	sb.WriteString(helpStyle.Render("  " + help))

	return sb.String()
}

// Result returns the wizard result after the model quits.
func (m wizardModel) Result() *WizardResult {
	if !m.launching {
		return nil
	}

	agents := m.agents.EnabledAgents()
	if len(agents) == 0 {
		return nil
	}

	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{
			Name: m.project.ProjectName(),
			Cwd:  m.project.ProjectPath(),
		},
		Agents: agents,
	}

	return &WizardResult{
		Config: cfg,
		Save:   m.saving,
	}
}
