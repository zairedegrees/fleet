package wizard

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

var errCancelled = fmt.Errorf("cancelled")

// Shared styles used across all wizard panels.
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// WizardResult is the outcome of the interactive wizard.
type WizardResult struct {
	Config *config.FleetConfig
	Save   bool
}

// Run executes the one-screen interactive wizard.
func Run(relayClient *relay.Client) (*WizardResult, error) {
	m := newWizardModel(relayClient)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("wizard: %w", err)
	}

	fm := final.(wizardModel)
	if fm.quitting {
		return nil, errCancelled
	}

	result := fm.Result()
	if result == nil {
		return nil, errCancelled
	}

	return result, nil
}
