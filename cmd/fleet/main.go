package main

import (
	"fmt"
	"os"
	"strings"
	"time"

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

	dispatchCmd := &cobra.Command{
		Use:   "dispatch [description]",
		Short: "Dispatch a task to an agent and wake it",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runDispatch,
	}
	dispatchCmd.Flags().String("to", "", "Agent to dispatch to (required)")
	dispatchCmd.MarkFlagRequired("to")
	root.AddCommand(dispatchCmd)

	logsCmd := &cobra.Command{
		Use:   "logs <agent>",
		Short: "Stream an agent's terminal output",
		Args:  cobra.ExactArgs(1),
		RunE:  runLogs,
	}
	logsCmd.Flags().IntP("lines", "n", 50, "Number of lines to show")
	logsCmd.Flags().BoolP("follow", "f", true, "Follow output (poll every 1s)")
	root.AddCommand(logsCmd)

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add an agent to the running fleet",
		RunE:  runAdd,
	}
	addCmd.Flags().String("name", "", "Agent name (required)")
	addCmd.Flags().String("role", "", "Agent role (required)")
	addCmd.Flags().String("color", "green", "Agent color")
	addCmd.Flags().String("reports-to", "", "Manager agent name")
	addCmd.MarkFlagRequired("name")
	addCmd.MarkFlagRequired("role")
	root.AddCommand(addCmd)

	stopCmd := &cobra.Command{
		Use:   "stop <agent>",
		Short: "Stop a running agent",
		Args:  cobra.ExactArgs(1),
		RunE:  runStop,
	}
	root.AddCommand(stopCmd)

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

	// Auto-save current config before killing
	cfg, err := config.LoadLast()
	if err == nil {
		config.SaveAsLast(cfg)
		fmt.Printf("  Config saved for %s.\n", cfg.Project.Name)
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

func runDispatch(cmd *cobra.Command, args []string) error {
	agent, _ := cmd.Flags().GetString("to")
	description := strings.Join(args, " ")

	cfg, err := config.LoadLast()
	if err != nil {
		return fmt.Errorf("no fleet config found. Run 'fleet' first")
	}

	relayURL := cfg.Project.RelayURL
	if relayURL == "" {
		relayURL = defaultRelayURL
	}

	client := relay.NewClient(relayURL)
	if err := client.DispatchTask(agent, cfg.Project.Name, description); err != nil {
		return fmt.Errorf("dispatch failed: %w", err)
	}

	if err := runner.WakeAgent(agent); err != nil {
		return fmt.Errorf("wake failed: %w", err)
	}

	fmt.Printf("  ✓ Task dispatched to %s and agent woken\n", agent)
	return nil
}

func runLogs(cmd *cobra.Command, args []string) error {
	agent := args[0]
	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")

	if !runner.HasSession(agent) {
		return fmt.Errorf("no fleet session for agent %q. Run 'fleet --status' to see active sessions", agent)
	}

	// Initial capture
	output, err := runner.CapturePane(agent)
	if err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}

	// Show last N lines
	allLines := strings.Split(output, "\n")
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}
	lastOutput := strings.Join(allLines[start:], "\n")
	fmt.Print(lastOutput)

	if !follow {
		return nil
	}

	// Follow mode: poll every 1s, print new content
	prev := output
	for {
		time.Sleep(1 * time.Second)
		current, err := runner.CapturePane(agent)
		if err != nil {
			return nil // session probably died
		}
		if current != prev {
			// Print the diff — find where new content starts
			// Simple approach: just reprint everything if changed
			fmt.Print("\033[2J\033[H") // clear screen
			allLines = strings.Split(current, "\n")
			start = 0
			if len(allLines) > lines {
				start = len(allLines) - lines
			}
			fmt.Print(strings.Join(allLines[start:], "\n"))
			prev = current
		}
	}
}

func runAdd(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	role, _ := cmd.Flags().GetString("role")
	color, _ := cmd.Flags().GetString("color")
	reportsTo, _ := cmd.Flags().GetString("reports-to")

	cfg, err := config.LoadLast()
	if err != nil {
		return fmt.Errorf("no fleet config found. Run 'fleet' first")
	}

	if runner.HasSession(name) {
		return fmt.Errorf("agent %q already has a running session", name)
	}

	agent := config.AgentConfig{
		Name:      name,
		Color:     color,
		Role:      role,
		ReportsTo: reportsTo,
	}

	// Create tmux session + launch claude
	claudeCmd := "claude"
	for _, f := range cfg.Claude.Flags {
		claudeCmd += " " + f
	}

	if err := runner.CreateSession(name, cfg.Project.Cwd); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	if err := runner.SendKeys(name, claudeCmd); err != nil {
		return fmt.Errorf("failed to launch claude: %w", err)
	}

	// Append to config and save
	cfg.Agents = append(cfg.Agents, agent)
	if err := config.SaveAsLast(cfg); err != nil {
		fmt.Printf("  ⚠ Failed to update config: %v\n", err)
	}

	fmt.Printf("  ✓ Agent %s added. Configure manually or use 'fleet dispatch' to assign work.\n", name)
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	agent := args[0]

	if !runner.HasSession(agent) {
		return fmt.Errorf("no fleet session for agent %q", agent)
	}

	// Try graceful exit first
	runner.SendKeys(agent, "/exit")
	time.Sleep(3 * time.Second)

	// Force kill if still alive
	if runner.HasSession(agent) {
		runner.KillSession(agent)
	}

	// Update config if possible
	cfg, err := config.LoadLast()
	if err == nil {
		var updated []config.AgentConfig
		for _, a := range cfg.Agents {
			if a.Name != agent {
				updated = append(updated, a)
			}
		}
		cfg.Agents = updated
		config.SaveAsLast(cfg)
	}

	// Check if fleet is now empty
	sessions, _ := runner.ListFleetSessions()
	if len(sessions) == 0 {
		fmt.Printf("  ✓ Agent %s stopped. Fleet is now empty.\n", agent)
	} else {
		fmt.Printf("  ✓ Agent %s stopped. %d agents remaining.\n", agent, len(sessions))
	}
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
	// Validate config
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Health check: relay must be reachable
	relayClient := relay.NewClient(cfg.Project.RelayURL)
	if err := relayClient.Health(); err != nil {
		fmt.Printf("  ✗ Relay unreachable at %s\n", cfg.Project.RelayURL)
		fmt.Println("    Run 'fleet --doctor' to check prerequisites.")
		return fmt.Errorf("relay health check failed: %w", err)
	}

	// Always save config — needed for --last, --kill, add, stop, dispatch
	if err := config.SaveAsLast(cfg); err != nil {
		fmt.Printf("  ⚠ Failed to save config: %v\n", err)
	} else if save {
		fmt.Println("  Config saved to ~/.fleet/configs/" + cfg.Project.Name + ".toml")
	}

	fmt.Printf("\n  🚀 Launching fleet...\n\n")

	results := runner.CreateSessions(cfg)

	success := 0
	for _, r := range results {
		if r.Success {
			success++
		}
	}

	var agentNames []string
	for _, a := range cfg.Agents {
		agentNames = append(agentNames, a.Name)
	}
	runner.OpenITerm2Grid(agentNames)

	runner.ConfigureAgentsAsync(cfg)

	fmt.Printf("\n  ✅ Fleet launched. %d/%d sessions created.\n", success, len(cfg.Agents))
	fmt.Printf("  Agents configuring in background (watch iTerm2 panes).\n\n")
	return nil
}
