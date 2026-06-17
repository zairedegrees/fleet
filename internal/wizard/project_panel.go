package wizard

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zairedegrees/fleet/internal/config"
)

// deriveProjectName turns a chosen filesystem path into a project name that is
// safe to interpolate into the generated tmux/shell/AppleScript and that passes
// config.Validate() — folders like "site.com" or "My App" would otherwise fail.
func deriveProjectName(path string) string {
	return config.NormalizeProjectName(filepath.Base(path))
}

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
	focusRelayURL    leftFocus = 4
	focusSettings    leftFocus = 5
)

type projectPanel struct {
	// Existing projects
	existingProjects []existingProject
	projectCursor    int

	// Path input (for new projects)
	pathInput textinput.Model
	projName  string // auto-derived from path basename

	// Relay URL input, prefilled with the default relay
	relayInput textinput.Model
	relayErr   string // validation error for the relay URL step

	// Presets
	presets      []Preset
	presetCursor int

	// Settings hub cursor: 0=Path, 1=Relay, 2=Team
	settingsCursor int

	focus leftFocus
	ready bool
	width int
}

type existingProject struct {
	name    string
	path    string    // cwd from saved config, empty if unknown
	modTime time.Time // mtime of configs/<name>.toml; zero when config-less
}

func newProjectPanel() projectPanel {
	pi := textinput.New()
	pi.Placeholder = "~/path/to/project"
	pi.CharLimit = 200
	pi.Width = 30
	pi.ShowSuggestions = true

	ri := textinput.New()
	ri.Placeholder = config.DefaultRelayURL
	ri.CharLimit = 200
	ri.Width = 30
	ri.SetValue(config.DefaultRelayURL)

	existing := discoverProjects()

	// Pre-set the cursor on the last-launched project (last.toml target). Falls
	// back to 0 (the most recent by mtime) when there is no last.toml.
	cursor := 0
	if cfg, err := config.LoadLast(); err == nil {
		for i, ep := range existing {
			if ep.name == cfg.Project.Name {
				cursor = i
				break
			}
		}
	}

	return projectPanel{
		pathInput:        pi,
		relayInput:       ri,
		presets:          AllPresets(),
		existingProjects: existing,
		projectCursor:    cursor,
		focus:            focusProjectList,
	}
}

// discoverProjects finds existing projects from configs and projects file,
// most-recently-used first (by config file mtime). Config-less projects (from
// the projects file) have no mtime and sink to the bottom.
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
					var mt time.Time
					if info, err := e.Info(); err == nil {
						mt = info.ModTime()
					}
					path := ""
					cfgPath := filepath.Join(configDir, e.Name())
					if cfg, err := config.Load(cfgPath); err == nil {
						path = cfg.Project.Cwd
					}
					projects = append(projects, existingProject{name: name, path: path, modTime: mt})
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

	// 3. Check last.toml symlink. Normally last.toml → configs/<name>.toml, so
	// the configs scan above already added this project and `seen` skips it.
	// This branch is the orphan-recovery path: the config was deleted but
	// last.toml still resolves. A dangling symlink makes os.Stat error, leaving
	// mt zero (sinks to bottom), which is fine.
	lastPath := filepath.Join(config.FleetDir(), "last.toml")
	if cfg, err := config.Load(lastPath); err == nil {
		if !seen[cfg.Project.Name] {
			seen[cfg.Project.Name] = true
			var mt time.Time
			if info, err := os.Stat(lastPath); err == nil { // follows symlink → target mtime
				mt = info.ModTime()
			}
			projects = append(projects, existingProject{name: cfg.Project.Name, path: cfg.Project.Cwd, modTime: mt})
		}
	}

	// Most-recent first; config-less (zero mtime) last; tie-break by name.
	sort.SliceStable(projects, func(i, j int) bool {
		ti, tj := projects[i].modTime, projects[j].modTime
		if ti.IsZero() != tj.IsZero() {
			return !ti.IsZero()
		}
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return projects[i].name < projects[j].name
	})

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
		case focusSettings:
			return p.updateSettings(msg)
		case focusPath:
			return p.updatePathInput(msg)
		case focusRelayURL:
			return p.updateRelayInput(msg)
		case focusPresets:
			return p.updatePresetList(msg)
		}
	}

	var cmd tea.Cmd
	if p.focus == focusPath {
		p.pathInput, cmd = p.pathInput.Update(msg)
	} else if p.focus == focusRelayURL {
		p.relayInput, cmd = p.relayInput.Update(msg)
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

// updateSettings drives the settings hub: a 3-row column (Path/Relay/Team) the
// user navigates with j/k and dives into with enter. esc is handled upstream in
// wizard_model (the non-text esc ladder).
func (p projectPanel) updateSettings(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.settingsCursor > 0 {
			p.settingsCursor--
		}
	case "down", "j":
		if p.settingsCursor < 2 {
			p.settingsCursor++
		}
	case "enter":
		switch p.settingsCursor {
		case 0: // Path
			p.focus = focusPath
			p.pathInput.Focus()
			return p, textinput.Blink
		case 1: // Relay
			p.focus = focusRelayURL
			p.relayInput.Focus()
			return p, textinput.Blink
		case 2: // Team
			p.focus = focusPresets
			return p, nil
		}
	}
	return p, nil
}

func (p projectPanel) updatePathInput(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.pathInput.Blur()
		if p.ready { // editing from the hub
			p.focus = focusSettings
			p.settingsCursor = 0
			return p, nil
		}
		p.focus = focusProjectList // new project, aborted
		return p, nil
	case "enter":
		pathVal := strings.TrimSpace(p.pathInput.Value())
		if pathVal == "" {
			return p, nil
		}
		expanded := expandHome(pathVal)

		// Only derive name from path for new projects (projName not already set)
		if p.projName == "" {
			p.projName = deriveProjectName(expanded)
		}

		// Create directory if it doesn't exist
		os.MkdirAll(expanded, 0755)

		p.pathInput.Blur()
		if p.ready { // hub edit → straight back to the hub
			p.focus = focusSettings
			p.settingsCursor = 0
			return p, nil
		}
		// New-project linear flow: path → relay.
		p.focus = focusRelayURL
		p.relayInput.Focus()
		return p, textinput.Blink
	}

	var cmd tea.Cmd
	p.pathInput, cmd = p.pathInput.Update(msg)
	p.pathInput.SetSuggestions(pathSuggestions(p.pathInput.Value()))
	return p, cmd
}

func (p projectPanel) updateRelayInput(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.relayErr = ""
		p.relayInput.Blur()
		if p.ready { // editing from the hub
			p.focus = focusSettings
			p.settingsCursor = 1
			return p, nil
		}
		// New-project linear flow: step back to path.
		p.focus = focusPath
		p.pathInput.Focus()
		return p, textinput.Blink
	case "enter":
		// Validate here, on submit: a typo'd URL would otherwise only fail
		// at launch time, after the wizard has exited and the config is gone.
		// Empty still means "use the default" (see RelayURL).
		if val := strings.TrimSpace(p.relayInput.Value()); val != "" {
			if err := validateRelayURL(val); err != nil {
				p.relayErr = err.Error()
				return p, nil
			}
		}
		p.relayErr = ""
		p.relayInput.Blur()
		if p.ready { // hub edit → back to the Relay row
			p.focus = focusSettings
			p.settingsCursor = 1
			return p, nil
		}
		// New-project linear flow: relay confirmed → project is ready; land in
		// the hub on the Team row to nudge picking a team.
		p.ready = true
		p.focus = focusSettings
		p.settingsCursor = 2
		return p, nil
	}

	var cmd tea.Cmd
	p.relayInput, cmd = p.relayInput.Update(msg)
	return p, cmd
}

func validateRelayURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid relay URL: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("relay URL must start with http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("relay URL needs a host, e.g. %s", config.DefaultRelayURL)
	}
	return nil
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

func (p projectPanel) View(active bool, agentCount int) string {
	var sb strings.Builder

	borderColor := lipgloss.Color("238")
	if active {
		borderColor = lipgloss.Color("99")
	}

	sb.WriteString(dimStyle.Render("PROJECT") + "\n")

	switch p.focus {
	case focusProjectList:
		for i, proj := range p.existingProjects {
			cursor := "  "
			style := dimStyle
			if i == p.projectCursor {
				cursor = selectedStyle.Render("▸ ")
				style = selectedStyle
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
		idx := len(p.existingProjects)
		cursor := "  "
		style := dimStyle
		if p.projectCursor == idx {
			cursor = selectedStyle.Render("▸ ")
			style = selectedStyle
		}
		sb.WriteString(cursor + style.Render("+ New project...") + "\n")

	case focusPath:
		sb.WriteString("  Path: " + p.pathInput.View() + "\n")
		val := strings.TrimSpace(p.pathInput.Value())
		if val != "" {
			name := filepath.Base(expandHome(val))
			sb.WriteString("  " + dimStyle.Render("Name: ") + selectedStyle.Render(name) + dimStyle.Render(" (auto)") + "\n")
		}

	case focusRelayURL:
		sb.WriteString("  " + dimStyle.Render("Path: ") + selectedStyle.Render(p.pathInput.Value()) + "\n")
		sb.WriteString("  Relay: " + p.relayInput.View() + "\n")
		if p.relayErr != "" {
			sb.WriteString("  " + errorStyle.Render("⚠ "+p.relayErr) + "\n")
		}

	case focusSettings:
		home, _ := os.UserHomeDir()
		pathDisp := p.pathInput.Value()
		if home != "" && strings.HasPrefix(pathDisp, home) {
			pathDisp = "~" + pathDisp[len(home):]
		}
		rows := []struct{ label, val string }{
			{"Path:  ", pathDisp},
			{"Relay: ", p.RelayURL()},
			{"Team:  ", fmt.Sprintf("%d agents", agentCount)},
		}
		for i, r := range rows {
			cursor := "  "
			vstyle := dimStyle
			if i == p.settingsCursor {
				cursor = selectedStyle.Render("▸ ")
				vstyle = selectedStyle
			}
			sb.WriteString(cursor + dimStyle.Render(r.label) + vstyle.Render(r.val) + "\n")
		}

	case focusPresets:
		sb.WriteString("  " + dimStyle.Render("Name: ") + selectedStyle.Render(p.projName) + "\n")
		sb.WriteString("  " + dimStyle.Render("Path: ") + selectedStyle.Render(p.pathInput.Value()) + "\n")
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("CHOOSE TEAM") + "\n")
		for i, preset := range p.presets {
			cursor := "  "
			style := dimStyle
			if i == p.presetCursor {
				cursor = selectedStyle.Render("▸ ")
				style = selectedStyle
			}
			count := fmt.Sprintf("(%d)", len(preset.Agents))
			sb.WriteString(cursor + style.Render(preset.Icon+" "+preset.Name) + " " + dimStyle.Render(count) + "\n")
		}

	default:
		sb.WriteString("  " + dimStyle.Render("Name: ") + selectedStyle.Render(p.projName) + "\n")
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

// RelayURL returns the relay URL to persist in the config; an emptied field
// falls back to the default so the saved config stays launchable.
func (p projectPanel) RelayURL() string {
	if val := strings.TrimSpace(p.relayInput.Value()); val != "" {
		return val
	}
	return config.DefaultRelayURL
}

func (p projectPanel) IsReady() bool {
	return p.ready
}
