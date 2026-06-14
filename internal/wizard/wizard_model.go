package wizard

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
	"github.com/zairedegrees/fleet/internal/scanner"
	"github.com/zairedegrees/fleet/internal/term"
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
	status      string // surfaced relay/scan failure, shown in View
	skipPerms   bool   // fleet-wide --dangerously-skip-permissions, toggled with P

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
	return nil // starts on project list, no text input to focus
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
		// Existing project with saved config
		cfg := msg.Config
		m.project.projName = cfg.Project.Name
		m.project.pathInput.SetValue(cfg.Project.Cwd)
		if cfg.Project.RelayURL != "" {
			m.project.relayInput.SetValue(cfg.Project.RelayURL)
		}
		m.project.ready = true
		m.project.focus = focusPresets
		// Load agents from saved config
		var items []agentItem
		for _, a := range cfg.Agents {
			items = append(items, agentItem{agent: a, enabled: true})
		}
		// Also try relay for any agents not in the config
		if m.relayClient != nil {
			relayAgents, err := m.relayClient.ListAgents(cfg.Project.Name)
			if err != nil {
				m.status = "relay: " + term.Sanitize(err.Error())
			} else {
				seen := make(map[string]bool)
				for _, item := range items {
					seen[item.agent.Name] = true
				}
				for _, ra := range relayAgents {
					if !seen[ra.Name] {
						color := ra.Color
						if color == "" {
							color = agentColors[len(items)%len(agentColors)]
						}
						items = append(items, agentItem{
							agent: config.AgentConfig{
								Name: term.Sanitize(ra.Name), Color: color, Role: term.Sanitize(ra.Role),
								ReportsTo: term.Sanitize(ra.ReportsTo), IsExecutive: ra.IsExecutive,
							},
							enabled: true,
						})
					}
				}
			}
		}
		m.agents.SetAgents(items)
		m.activePanel = panelRight
		return m, nil

	case ProjectSelectedMsg:
		// Project known but no saved config — query relay for agents
		m.project.projName = msg.Name
		if msg.Path != "" {
			display := msg.Path
			if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(display, home) {
				display = "~" + display[len(home):]
			}
			m.project.pathInput.SetValue(display)
			m.project.ready = true
			m.project.focus = focusPresets
		} else {
			m.project.focus = focusPath
			m.project.pathInput.Focus()
		}
		// Query relay for existing agents
		if m.relayClient != nil {
			relayAgents, err := m.relayClient.ListAgents(msg.Name)
			if err != nil {
				m.status = "relay: " + term.Sanitize(err.Error())
			} else if len(relayAgents) > 0 {
				var items []agentItem
				for i, ra := range relayAgents {
					color := ra.Color
					if color == "" {
						color = agentColors[i%len(agentColors)]
					}
					items = append(items, agentItem{
						agent: config.AgentConfig{
							Name: term.Sanitize(ra.Name), Color: color, Role: term.Sanitize(ra.Role),
							ReportsTo: term.Sanitize(ra.ReportsTo), IsExecutive: ra.IsExecutive,
						},
						enabled: true,
					})
				}
				m.agents.SetAgents(items)
			}
		}
		if m.project.ready {
			m.activePanel = panelRight
		}
		return m, nil

	case PresetSelectedMsg:
		// Preset chosen in left panel -> populate right panel
		preset := msg.Preset
		if preset.Name == "Custom" {
			// Run scanner on project path
			path := m.project.ProjectPath()
			if path != "" {
				result, err := scanner.Scan(path)
				if err != nil {
					m.status = "scan: " + term.Sanitize(err.Error())
				} else {
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
		m.drawer.skipPerms = m.skipPerms
		m.drawerOpen = true
		m.agents.compressed = true
		return m, nil

	case NewAgentMsg:
		var names []string
		for _, item := range m.agents.items {
			names = append(names, item.agent.Name)
		}
		m.drawer.OpenCreate(names, len(m.agents.items))
		m.drawer.skipPerms = m.skipPerms
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
		isTextInput := (m.activePanel == panelLeft && (m.project.focus == focusPath || m.project.focus == focusRelayURL)) || m.drawerOpen

		if !isTextInput {
			switch keyMsg.String() {
			case "q":
				m.quitting = true
				return m, tea.Quit
			case "esc":
				// On the presets focus, esc steps back to the relay URL
				// field (delegated below) — the only way to edit a loaded
				// project's saved relay_url. Everywhere else it quits.
				if m.activePanel == panelLeft && m.project.focus == focusPresets {
					break
				}
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
			case "P":
				// Toggle fleet-wide full autonomy. Agents panel only — it's the
				// finalize screen before launch; the flag applies to the whole fleet.
				if m.activePanel == panelRight {
					m.skipPerms = !m.skipPerms
					return m, nil
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

	// Autonomy posture — fleet-wide permission stance, toggled with P.
	if m.skipPerms {
		autonomyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		sb.WriteString(autonomyStyle.Render("  ⚠ Autonomy: SKIP-ALL — agents skip every permission prompt") + "\n")
	} else {
		sb.WriteString(dimStyle.Render("  Autonomy: prompts  (P: skip all permissions)") + "\n")
	}

	// Status / error line — surfaces relay/scan failures that were swallowed.
	if m.status != "" {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		sb.WriteString(statusStyle.Render("  ⚠ "+m.status) + "\n")
	}

	// Help bar
	var help string
	if m.drawerOpen {
		help = "tab=field  j/k=select  enter=save  esc=cancel"
	} else if m.project.focus == focusProjectList {
		help = "j/k=move  enter=select  q=quit"
	} else if m.activePanel == panelLeft && m.project.focus == focusPath {
		help = "type path  tab=autocomplete  enter=confirm  esc=back  ctrl+c=quit"
	} else if m.activePanel == panelLeft && m.project.focus == focusRelayURL {
		help = "type relay URL  enter=confirm  esc=back  ctrl+c=quit"
	} else if m.activePanel == panelLeft {
		help = "j/k=move  enter=select preset  esc=relay URL  tab=agents panel  q=quit"
	} else {
		help = "j/k=move  space=toggle  e=edit  n=new  d=del  a=all  P=autonomy  enter=launch  s=save+launch  tab=presets  q=quit"
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
			Name:     m.project.ProjectName(),
			RelayURL: m.project.RelayURL(),
			Cwd:      m.project.ProjectPath(),
		},
		Agents: agents,
	}

	if m.skipPerms {
		cfg.Claude.Flags = []string{"--dangerously-skip-permissions"}
	}

	return &WizardResult{
		Config: cfg,
		Save:   m.saving,
	}
}
