package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nazaire/fleet/internal/config"
	"github.com/nazaire/fleet/internal/doctor"
	"github.com/nazaire/fleet/internal/relay"
	"github.com/nazaire/fleet/internal/runner"
	"github.com/nazaire/fleet/internal/wizard"
)

const defaultRelayURL = "http://localhost:8090/mcp"

var (
	flagLast   bool
	flagKill   bool
	flagStatus bool
	flagDoctor bool
)

func main() {
	root := &cobra.Command{
		Use:   "fleet",
		Short: "⚡ Launch multi-agent Claude Code fleets",
		RunE:  run,
	}

	root.Flags().BoolVar(&flagLast, "last", false, "Relaunch last saved config")
	root.Flags().BoolVar(&flagKill, "kill", false, "Stop all fleet tmux sessions")
	root.Flags().BoolVar(&flagStatus, "status", false, "List active fleet sessions")
	root.Flags().BoolVar(&flagDoctor, "doctor", false, "Check & install prerequisites")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	switch {
	case flagDoctor:
		return runDoctor()
	case flagKill:
		return runKill()
	case flagStatus:
		return runStatus()
	case flagLast:
		return runLast()
	default:
		return runWizard()
	}
}

func runDoctor() error {
	checks := doctor.Run()
	doctor.Print(checks)
	return nil
}

func runKill() error {
	sessions, _ := runner.ListFleetSessions()
	if len(sessions) == 0 {
		fmt.Println("  No fleet sessions running.")
		return nil
	}
	runner.KillAllFleetSessions()
	fmt.Printf("  Killed %d fleet sessions.\n", len(sessions))
	return nil
}

func runStatus() error {
	sessions, _ := runner.ListFleetSessions()
	if len(sessions) == 0 {
		fmt.Println("  No fleet sessions running.")
		return nil
	}
	fmt.Printf("  %d fleet sessions:\n\n", len(sessions))
	for _, s := range sessions {
		idle := "busy"
		agent := s[3:] // strip "pm-"
		if runner.IsIdle(agent) {
			idle = "idle"
		}
		fmt.Printf("    %s  [%s]\n", s, idle)
	}
	fmt.Println()
	return nil
}

func runWizard() error {
	client := relay.NewClient(defaultRelayURL)

	result, err := wizard.Run(client)
	if err != nil {
		if err.Error() == "cancelled" {
			return nil
		}
		return err
	}

	if result.Config.Project.RelayURL == "" {
		result.Config.Project.RelayURL = defaultRelayURL
	}

	return launch(result.Config, result.Save)
}

func runLast() error {
	cfg, err := config.LoadLast()
	if err != nil {
		fmt.Println("  No saved config found. Run 'fleet' to create one.")
		return runWizard()
	}

	if cfg.Project.RelayURL == "" {
		cfg.Project.RelayURL = defaultRelayURL
	}

	// Check for existing sessions
	var agentNames []string
	for _, a := range cfg.Agents {
		agentNames = append(agentNames, a.Name)
	}
	conflicts := runner.DetectConflicts(agentNames)

	running := 0
	for _, c := range conflicts {
		if c.HasTmux {
			running++
		}
	}

	if running == len(cfg.Agents) {
		fmt.Printf("  Fleet %s is already running (%d agents).\n", cfg.Project.Name, running)
		fmt.Println("  Use 'fleet --status' to check or 'fleet --kill' to stop.")
		return nil
	}

	if running > 0 {
		fmt.Printf("  ⚠ %d/%d agents already running:\n", running, len(cfg.Agents))
		for _, c := range conflicts {
			if c.HasTmux {
				state := "busy"
				if c.IsIdle {
					state = "idle"
				}
				fmt.Printf("    %s [%s]\n", c.Name, state)
			}
		}
		fmt.Println()
		fmt.Println("  Continuing will skip existing sessions and create missing ones.")
	}

	fmt.Printf("  ⚡ Relaunching %s (%d agents)\n\n", cfg.Project.Name, len(cfg.Agents))
	return launch(cfg, false)
}

func launch(cfg *config.FleetConfig, save bool) error {
	if save {
		if err := config.SaveAsLast(cfg); err != nil {
			fmt.Printf("  ⚠ Failed to save config: %v\n", err)
		} else {
			fmt.Println("  Config saved to ~/.fleet/configs/" + cfg.Project.Name + ".toml")
		}
	}

	fmt.Println("\n  🚀 Launching fleet...\n")

	// Phase 1: Create tmux sessions + launch claude (fast)
	results := runner.CreateSessions(cfg)

	success := 0
	for _, r := range results {
		if r.Success {
			success++
		}
	}

	// Phase 2: Open iTerm2 grid immediately (user sees panes while claude boots)
	var agentNames []string
	for _, a := range cfg.Agents {
		agentNames = append(agentNames, a.Name)
	}
	runner.OpenITerm2Grid(agentNames)

	// Phase 3: Configure agents in background via a shell script
	// (fleet exits, script waits for prompts and sends init commands)
	runner.ConfigureAgentsAsync(cfg)

	fmt.Printf("\n  ✅ Fleet launched. %d/%d sessions created.\n", success, len(cfg.Agents))
	fmt.Println("  Agents configuring in background (watch iTerm2 panes).\n")
	return nil
}
