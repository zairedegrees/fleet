package wizard

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/nazaire/fleet/internal/config"
	"github.com/nazaire/fleet/internal/relay"
)

var errCancelled = fmt.Errorf("cancelled")

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

// Run executes the interactive wizard: project selection, cwd, optional scan,
// agent selection, and confirmation. Returns the assembled FleetConfig.
func Run(relayClient *relay.Client) (*WizardResult, error) {
	// Step 1: Select project
	project, isNew, err := runProjectStep(relayClient)
	if err != nil {
		return nil, err
	}

	// Step 2: Project directory
	cwd, err := runCwdStep(project)
	if err != nil {
		return nil, err
	}

	// Step 3: Scan project (only for new projects)
	var suggestedAgents []config.AgentConfig
	if isNew {
		suggestedAgents, err = runScanStep(cwd)
		if err != nil {
			return nil, err
		}
	}

	// Step 4: Select/create agents (pre-filled with scan suggestions if available)
	agents, err := runAgentsStep(relayClient, project, isNew, suggestedAgents)
	if err != nil {
		return nil, err
	}

	// Step 5: Confirm
	result, err := runConfirmStep(project, agents)
	if err != nil {
		return nil, err
	}

	result.Config.Project.Cwd = cwd
	return result, nil
}
