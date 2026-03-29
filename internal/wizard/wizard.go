package wizard

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/nazaire/fleet/internal/config"
	"github.com/nazaire/fleet/internal/relay"
)

// Shared styles used across all wizard steps.
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

// Run executes the interactive wizard: project selection, agent selection,
// and confirmation. It returns the assembled FleetConfig and whether the
// user chose to persist it to disk.
func Run(relayClient *relay.Client) (*WizardResult, error) {
	// Step 1: Select project
	project, isNew, err := runProjectStep(relayClient)
	if err != nil {
		return nil, err
	}

	// Step 2: Select/create agents
	agents, err := runAgentsStep(relayClient, project, isNew)
	if err != nil {
		return nil, err
	}

	// Step 3: Confirm
	result, err := runConfirmStep(project, agents)
	if err != nil {
		return nil, err
	}

	return result, nil
}
