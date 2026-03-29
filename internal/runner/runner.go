package runner

import (
	"fmt"
	"time"

	"github.com/nazaire/fleet/internal/config"
)

type LaunchResult struct {
	Agent   string
	Success bool
	Error   error
}

func Launch(cfg *config.FleetConfig) ([]LaunchResult, error) {
	var results []LaunchResult

	claudeCmd := "claude"
	if len(cfg.Claude.Flags) > 0 {
		claudeCmd = "claude"
		for _, f := range cfg.Claude.Flags {
			claudeCmd += " " + f
		}
	}

	// Phase 1: Create tmux sessions and launch Claude Code
	for _, agent := range cfg.Agents {
		res := LaunchResult{Agent: agent.Name}

		if HasSession(agent.Name) {
			fmt.Printf("  ✓ %s already running, skipping\n", SessionName(agent.Name))
			res.Success = true
			results = append(results, res)
			continue
		}

		if err := CreateSession(agent.Name, cfg.Project.Cwd); err != nil {
			res.Error = fmt.Errorf("tmux create failed: %w", err)
			results = append(results, res)
			continue
		}

		if err := SendKeys(agent.Name, claudeCmd); err != nil {
			res.Error = fmt.Errorf("claude launch failed: %w", err)
			results = append(results, res)
			continue
		}

		res.Success = true
		results = append(results, res)
	}

	// Phase 2: Wait for all Claude instances to boot
	fmt.Println("  ⏳ Waiting for Claude Code to initialize...")
	for _, agent := range cfg.Agents {
		if !HasSession(agent.Name) {
			continue
		}
		if err := WaitForPrompt(agent.Name, 30*time.Second); err != nil {
			fmt.Printf("  ⚠ %s: timeout waiting for prompt\n", agent.Name)
		}
	}

	// Phase 3: Configure each agent
	for _, agent := range cfg.Agents {
		if !HasSession(agent.Name) {
			continue
		}

		SendKeys(agent.Name, "/rename "+agent.Name)
		WaitForPrompt(agent.Name, 10*time.Second)

		SendKeys(agent.Name, "/color "+agent.Color)
		WaitForPrompt(agent.Name, 10*time.Second)

		registerCmd := fmt.Sprintf("/relay register %s %s %s",
			agent.Name, cfg.Project.Name, agent.Role)
		if agent.ReportsTo != "" {
			registerCmd += " Reports to " + agent.ReportsTo + "."
		}
		SendKeys(agent.Name, registerCmd)
		WaitForPrompt(agent.Name, 15*time.Second)

		// Start talk loop on workers (not executives)
		if !agent.IsExecutive {
			SendKeys(agent.Name, "/relay talk")
		}

		fmt.Printf("  ✓ %s configured\n", SessionName(agent.Name))
	}

	return results, nil
}
