package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nazaire/fleet/internal/config"
)

// PresetSelectedMsg is sent when a preset is selected.
type PresetSelectedMsg struct {
	Preset Preset
}

// ProjectLoadedMsg is sent when an existing project is selected.
type ProjectLoadedMsg struct {
	Config *config.FleetConfig
}

type projectPanel struct {
	// Existing projects
	existingProjects []string // project names from ~/.fleet/configs/
	projectCursor    int
	showExisting     bool // true when showing project list

	// New project inputs
	nameInput textinput.Model
	pathInput textinput.Model

	// Presets
	presets      []Preset
	presetCursor int

	focus leftFocus
	ready bool
	width int
}

const (
	focusProjectList leftFocus = 3
)

func newProjectPanel() projectPanel {
	ni := textinput.New()
	ni.Placeholder = "project-name"
	ni.CharLimit = 40
	ni.Width = 25

	pi := textinput.New()
	pi.Placeholder = "/path/to/project"
	pi.CharLimit = 200
	pi.Width = 25
	pi.ShowSuggestions = true

	// Discover existing projects
	existing := discoverProjects()

	p := projectPanel{
		nameInput:        ni,
		pathInput:        pi,
		presets:          AllPresets(),
		existingProjects: existing,
		showExisting:     len(existing) > 0,
	}

	if p.showExisting {
		p.focus = focusProjectList
	} else {
		p.focus = focusName
		p.nameInput.Focus()
	}

	return p
}

// discoverProjects scans ~/.fleet/configs/ for existing project configs.
func discoverProjects() []string {
	configDir := filepath.Join(config.FleetDir(), "configs")
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil
	}
	var projects []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".toml") {
			projects = append(projects, strings.TrimSuffix(e.Name(), ".toml"))
		}
	}
	return projects
}

// pathSuggestions returns directory suggestions for the current path input.
func pathSuggestions(current string) []string {
	if current == "" {
		return nil
	}

	// Expand ~
	expanded := current
	if strings.HasPrefix(expanded, "~/") {
		home, _ := os.UserHomeDir()
		expanded = filepath.Join(home, expanded[2:])
	}

	dir := expanded
	prefix := ""

	// If the path doesn't end with /, treat the last component as a prefix filter
	if !strings.HasSuffix(expanded, "/") {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var suggestions []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		fullPath := filepath.Join(dir, name)
		// Convert back to ~/... for display
		if home, err := os.UserHomeDir(); err == nil {
			if strings.HasPrefix(fullPath, home) {
				fullPath = "~" + fullPath[len(home):]
			}
		}
		suggestions = append(suggestions, fullPath)
	}
	return suggestions
}

func (p projectPanel) Update(msg tea.Msg) (projectPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch p.focus {
		case focusProjectList:
			return p.updateProjectList(msg)
		case focusName:
			return p.updateNameInput(msg)
		case focusPath:
			return p.updatePathInput(msg)
		case focusPresets:
			return p.updatePresetList(msg)
		}
	}

	// Forward to active text input for non-key messages (blink, etc.)
	var cmd tea.Cmd
	switch p.focus {
	case focusName:
		p.nameInput, cmd = p.nameInput.Update(msg)
	case focusPath:
		p.pathInput, cmd = p.pathInput.Update(msg)
	}
	return p, cmd
}

func (p projectPanel) updateProjectList(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	totalItems := len(p.existingProjects) + 1 // +1 for "Create new..."

	switch msg.String() {
	case "up", "k":
		if p.projectCursor > 0 {
			p.projectCursor--
		}
	case "down", "j":
		if p.projectCursor < totalItems-1 {
			p.projectCursor++
		}
	case "enter":
		if p.projectCursor == len(p.existingProjects) {
			// "Create new..." selected
			p.showExisting = false
			p.focus = focusName
			p.nameInput.Focus()
			return p, textinput.Blink
		}
		// Load existing project
		name := p.existingProjects[p.projectCursor]
		cfgPath := filepath.Join(config.FleetDir(), "configs", name+".toml")
		if cfg, err := config.Load(cfgPath); err == nil {
			return p, func() tea.Msg {
				return ProjectLoadedMsg{Config: cfg}
			}
		}
	}
	return p, nil
}

func (p projectPanel) updateNameInput(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if len(p.existingProjects) > 0 {
			// Go back to project list
			p.showExisting = true
			p.focus = focusProjectList
			p.nameInput.Blur()
			return p, nil
		}
	case "enter":
		if strings.TrimSpace(p.nameInput.Value()) == "" {
			return p, nil
		}
		p.focus = focusPath
		p.nameInput.Blur()
		p.pathInput.Focus()
		// Auto-fill path with cwd if empty
		if p.pathInput.Value() == "" {
			if cwd, err := os.Getwd(); err == nil {
				p.pathInput.SetValue(cwd)
			}
		}
		// Generate initial path suggestions
		p.pathInput.SetSuggestions(pathSuggestions(p.pathInput.Value()))
		return p, textinput.Blink
	}
	var cmd tea.Cmd
	p.nameInput, cmd = p.nameInput.Update(msg)
	return p, cmd
}

func (p projectPanel) updatePathInput(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Go back to name input
		p.focus = focusName
		p.pathInput.Blur()
		p.nameInput.Focus()
		return p, textinput.Blink
	case "enter":
		if strings.TrimSpace(p.pathInput.Value()) == "" {
			return p, nil
		}
		p.focus = focusPresets
		p.pathInput.Blur()
		p.ready = true
		return p, nil
	}

	var cmd tea.Cmd
	p.pathInput, cmd = p.pathInput.Update(msg)

	// Update suggestions after each keystroke
	p.pathInput.SetSuggestions(pathSuggestions(p.pathInput.Value()))

	return p, cmd
}

func (p projectPanel) updatePresetList(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.presetCursor > 0 {
			p.presetCursor--
		}
	case "down", "j":
		if p.presetCursor < len(p.presets)-1 {
			p.presetCursor++
		}
	case "enter":
		preset := p.presets[p.presetCursor]
		return p, func() tea.Msg {
			return PresetSelectedMsg{Preset: preset}
		}
	}
	return p, nil
}

func (p projectPanel) View(active bool) string {
	var sb strings.Builder

	borderColor := lipgloss.Color("238")
	if active {
		borderColor = lipgloss.Color("99")
	}

	// PROJECT section
	sb.WriteString(dimStyle.Render("PROJECT") + "\n")

	if p.showExisting && p.focus == focusProjectList {
		// Show existing projects list
		for i, name := range p.existingProjects {
			cursor := "  "
			style := dimStyle
			if i == p.projectCursor {
				cursor = selectedStyle.Render("▸ ")
				style = selectedStyle
			}
			sb.WriteString(cursor + style.Render(name) + "\n")
		}
		// "Create new..." sentinel
		idx := len(p.existingProjects)
		cursor := "  "
		style := dimStyle
		if p.projectCursor == idx {
			cursor = selectedStyle.Render("▸ ")
			style = selectedStyle
		}
		sb.WriteString(cursor + style.Render("+ Create new project...") + "\n")
	} else {
		// New project input or confirmed values
		if p.focus == focusName {
			sb.WriteString("  Name: " + p.nameInput.View() + "\n")
		} else {
			name := p.nameInput.Value()
			if name == "" {
				name = dimStyle.Render("(not set)")
			} else {
				name = selectedStyle.Render(name)
			}
			sb.WriteString("  " + dimStyle.Render("Name: ") + name + "\n")
		}

		if p.focus == focusPath {
			sb.WriteString("  Path: " + p.pathInput.View() + "\n")
		} else {
			path := p.pathInput.Value()
			if path == "" {
				path = dimStyle.Render("(not set)")
			} else {
				path = selectedStyle.Render(path)
			}
			sb.WriteString("  " + dimStyle.Render("Path: ") + path + "\n")
		}
	}

	sb.WriteString("\n")

	// PRESET section
	sb.WriteString(dimStyle.Render("PRESET") + "\n")

	if !p.ready {
		sb.WriteString(dimStyle.Render("  Set project first...") + "\n")
	} else {
		for i, preset := range p.presets {
			cursor := "  "
			style := dimStyle
			if p.focus == focusPresets && i == p.presetCursor {
				cursor = selectedStyle.Render("▸ ")
				style = selectedStyle
			}
			count := fmt.Sprintf("(%d)", len(preset.Agents))
			sb.WriteString(cursor + style.Render(preset.Icon+" "+preset.Name) + " " + dimStyle.Render(count) + "\n")
		}
	}

	// Apply border
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

func (p projectPanel) ProjectName() string {
	return strings.TrimSpace(p.nameInput.Value())
}

func (p projectPanel) ProjectPath() string {
	path := strings.TrimSpace(p.pathInput.Value())
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	return path
}

func (p projectPanel) IsReady() bool {
	return p.ready
}
