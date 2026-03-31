package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PresetSelectedMsg is sent when a preset is selected.
type PresetSelectedMsg struct {
	Preset Preset
}

// projectPanel is the left panel sub-model.
type projectPanel struct {
	nameInput    textinput.Model
	pathInput    textinput.Model
	presets      []Preset
	presetCursor int
	focus        leftFocus
	ready        bool // project name+path confirmed
	width        int
}

func newProjectPanel() projectPanel {
	ni := textinput.New()
	ni.Placeholder = "project-name"
	ni.Focus()
	ni.CharLimit = 40
	ni.Width = 25

	pi := textinput.New()
	pi.Placeholder = "~/code/project"
	pi.CharLimit = 100
	pi.Width = 25

	return projectPanel{
		nameInput: ni,
		pathInput: pi,
		presets:   AllPresets(),
		focus:     focusName,
	}
}

func (p projectPanel) Update(msg tea.Msg) (projectPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch p.focus {
		case focusName:
			return p.updateNameInput(msg)
		case focusPath:
			return p.updatePathInput(msg)
		case focusPresets:
			return p.updatePresetList(msg)
		}
	}

	// Forward to active text input
	var cmd tea.Cmd
	switch p.focus {
	case focusName:
		p.nameInput, cmd = p.nameInput.Update(msg)
	case focusPath:
		p.pathInput, cmd = p.pathInput.Update(msg)
	}
	return p, cmd
}

func (p projectPanel) updateNameInput(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if strings.TrimSpace(p.nameInput.Value()) == "" {
			return p, nil
		}
		p.focus = focusPath
		p.nameInput.Blur()
		p.pathInput.Focus()
		// Auto-fill path if empty
		if p.pathInput.Value() == "" {
			home, _ := os.UserHomeDir()
			p.pathInput.SetValue(filepath.Join(home, "code", p.nameInput.Value()))
		}
		return p, textinput.Blink
	}
	var cmd tea.Cmd
	p.nameInput, cmd = p.nameInput.Update(msg)
	return p, cmd
}

func (p projectPanel) updatePathInput(msg tea.KeyMsg) (projectPanel, tea.Cmd) {
	switch msg.String() {
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

// ProjectName returns the entered project name.
func (p projectPanel) ProjectName() string {
	return strings.TrimSpace(p.nameInput.Value())
}

// ProjectPath returns the entered project path.
func (p projectPanel) ProjectPath() string {
	return strings.TrimSpace(p.pathInput.Value())
}

// IsReady returns true if project name and path are confirmed.
func (p projectPanel) IsReady() bool {
	return p.ready
}
