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

// ProjectLoadedMsg is sent when an existing project with saved config is selected.
type ProjectLoadedMsg struct {
	Config *config.FleetConfig
}

// ProjectSelectedMsg is sent when a project is selected but has no saved config.
// The wizard model should query the relay for its agents.
type ProjectSelectedMsg struct {
	Name string
	Path string
}

const (
	focusProjectList leftFocus = 3
)

type projectPanel struct {
	// Existing projects
	existingProjects []existingProject
	projectCursor    int

	// Path input (for new projects)
	pathInput textinput.Model
	projName  string // auto-derived from path basename

	// Presets
	presets      []Preset
	presetCursor int

	focus leftFocus
	ready bool
	width int
}

type existingProject struct {
	name string
	path string // cwd from saved config, empty if unknown
}

func newProjectPanel() projectPanel {
	pi := textinput.New()
	pi.Placeholder = "~/path/to/project"
	pi.CharLimit = 200
	pi.Width = 30
	pi.ShowSuggestions = true

	existing := discoverProjects()

	return projectPanel{
		pathInput:        pi,
		presets:          AllPresets(),
		existingProjects: existing,
		focus:            focusProjectList,
	}
}

// discoverProjects finds existing projects from configs and projects file.
func discoverProjects() []existingProject {
	seen := make(map[string]bool)
	var projects []existingProject

	// 1. Scan ~/.fleet/configs/*.toml
	configDir := filepath.Join(config.FleetDir(), "configs")
	entries, err := os.ReadDir(configDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".toml") {
				name := strings.TrimSuffix(e.Name(), ".toml")
				if !seen[name] {
					seen[name] = true
					// Try to load the config to get the cwd
					path := ""
					cfgPath := filepath.Join(configDir, e.Name())
					if cfg, err := config.Load(cfgPath); err == nil {
						path = cfg.Project.Cwd
					}
					projects = append(projects, existingProject{name: name, path: path})
				}
			}
		}
	}

	// 2. Scan ~/.fleet/projects file (one name per line)
	projectsFile := filepath.Join(config.FleetDir(), "projects")
	data, err := os.ReadFile(projectsFile)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			name := strings.TrimSpace(line)
			if name != "" && !seen[name] {
				seen[name] = true
				projects = append(projects, existingProject{name: name})
			}
		}
	}

	// 3. Check last.toml symlink
	lastPath := filepath.Join(config.FleetDir(), "last.toml")
	if cfg, err := config.Load(lastPath); err == nil {
		if !seen[cfg.Project.Name] {
			seen[cfg.Project.Name] = true
			projects = append(projects, existingProject{name: cfg.Project.Name, path: cfg.Project.Cwd})
		}
	}

	return projects
}

// pathSuggestions returns directory suggestions for the current path input.
func pathSuggestions(current string) []string {
	home, _ := os.UserHomeDir()

	if current == "" {
		// Empty input — suggest home subdirectories
		if home == "" {
			return nil
		}
		return listDirSuggestions(home, "", home)
	}

	expanded := expandHome(current)
	dir := expanded
	prefix := ""

	if !strings.HasSuffix(expanded, "/") {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	return listDirSuggestions(dir, prefix, home)
}

func listDirSuggestions(dir, prefix, home string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var suggestions []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(e.Name()), strings.ToLower(prefix)) {
			continue
		}
		fullPath := filepath.Join(dir, e.Name())
		if home != "" && strings.HasPrefix(fullPath, home) {
			fullPath = "~" + fullPath[len(home):]
		}
		suggestions = append(suggestions, fullPath)
	}
	return suggestions
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func (p projectPanel) Update(msg tea.Msg) (projectPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch p.focus {
		case focusProjectList:
			return p.updateProjectList(msg)
		case focusPath:
			return p.updatePathInput(msg)
		case focusPresets:
			return p.updatePresetList(msg)
		}
	}

	var cmd tea.Cmd
	if p.focus == focusPath {
		p.pathInput, cmd = p.pathInput.Update(msg)
	}
	return p, cmd
}

func (p projectPanel) updateProjectList(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	totalItems := len(p.existingProjects) + 1 // +1 for "New project..."

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
			// "New project..." selected → show path input
			p.focus = focusPath
			if p.pathInput.Value() == "" {
				p.pathInput.SetValue("~/")
				p.pathInput.SetSuggestions(pathSuggestions("~/"))
			}
			p.pathInput.Focus()
			return p, textinput.Blink
		}
		// Load existing project
		proj := p.existingProjects[p.projectCursor]
		cfgPath := filepath.Join(config.FleetDir(), "configs", proj.name+".toml")
		if cfg, err := config.Load(cfgPath); err == nil {
			return p, func() tea.Msg {
				return ProjectLoadedMsg{Config: cfg}
			}
		}
		// Config file not found — emit selection for wizard_model to query relay
		// Pre-fill path with ~/ if no path known, so autocomplete works immediately
		if proj.path == "" && p.pathInput.Value() == "" {
			p.pathInput.SetValue("~/")
			p.pathInput.SetSuggestions(pathSuggestions("~/"))
		}
		return p, func() tea.Msg {
			return ProjectSelectedMsg{Name: proj.name, Path: proj.path}
		}
	}
	return p, nil
}

func (p projectPanel) updatePathInput(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.focus = focusProjectList
		p.pathInput.Blur()
		return p, nil
	case "enter":
		pathVal := strings.TrimSpace(p.pathInput.Value())
		if pathVal == "" {
			return p, nil
		}
		expanded := expandHome(pathVal)

		// Only derive name from path for new projects (projName not already set)
		if p.projName == "" {
			p.projName = filepath.Base(expanded)
		}

		// Create directory if it doesn't exist
		os.MkdirAll(expanded, 0755)

		p.focus = focusPresets
		p.pathInput.Blur()
		p.ready = true
		return p, nil
	}

	var cmd tea.Cmd
	p.pathInput, cmd = p.pathInput.Update(msg)
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

	sb.WriteString(dimStyle.Render("PROJECT") + "\n")

	if p.focus == focusProjectList {
		// Project list
		for i, proj := range p.existingProjects {
			cursor := "  "
			style := dimStyle
			if i == p.projectCursor {
				cursor = selectedStyle.Render("▸ ")
				style = selectedStyle
			}
			label := proj.name
			if proj.path != "" {
				home, _ := os.UserHomeDir()
				short := proj.path
				if home != "" && strings.HasPrefix(short, home) {
					short = "~" + short[len(home):]
				}
				label += " " + dimStyle.Render(short)
			}
			sb.WriteString(cursor + style.Render(proj.name))
			if proj.path != "" {
				home, _ := os.UserHomeDir()
				short := proj.path
				if home != "" && strings.HasPrefix(short, home) {
					short = "~" + short[len(home):]
				}
				sb.WriteString(" " + dimStyle.Render(short))
			}
			sb.WriteString("\n")
		}
		// "New project..." sentinel
		idx := len(p.existingProjects)
		cursor := "  "
		style := dimStyle
		if p.projectCursor == idx {
			cursor = selectedStyle.Render("▸ ")
			style = selectedStyle
		}
		sb.WriteString(cursor + style.Render("+ New project...") + "\n")
	} else if p.focus == focusPath {
		// Path input mode
		sb.WriteString("  Path: " + p.pathInput.View() + "\n")
		// Show auto-derived name preview
		val := strings.TrimSpace(p.pathInput.Value())
		if val != "" {
			name := filepath.Base(expandHome(val))
			sb.WriteString("  " + dimStyle.Render("Name: ") + selectedStyle.Render(name) + dimStyle.Render(" (auto)") + "\n")
		}
	} else {
		// Confirmed — show name + path
		sb.WriteString("  " + dimStyle.Render("Name: ") + selectedStyle.Render(p.projName) + "\n")
		sb.WriteString("  " + dimStyle.Render("Path: ") + selectedStyle.Render(p.pathInput.Value()) + "\n")
	}

	sb.WriteString("\n")
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
	if p.projName != "" {
		return p.projName
	}
	val := strings.TrimSpace(p.pathInput.Value())
	if val != "" {
		return filepath.Base(expandHome(val))
	}
	return ""
}

func (p projectPanel) ProjectPath() string {
	return expandHome(strings.TrimSpace(p.pathInput.Value()))
}

func (p projectPanel) IsReady() bool {
	return p.ready
}
