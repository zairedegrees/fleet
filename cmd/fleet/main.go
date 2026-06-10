package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/doctor"
	"github.com/zairedegrees/fleet/internal/relay"
	"github.com/zairedegrees/fleet/internal/runner"
	"github.com/zairedegrees/fleet/internal/wizard"
)

const defaultRelayURL = config.DefaultRelayURL

// loadLastConfig is a seam over config.LoadLast so command behavior around a
// missing/corrupt last config can be unit-tested.
var loadLastConfig = config.LoadLast

// Seams over the tmux layer so the kill-all confirmation flow is unit-testable
// without killing real sessions.
var (
	listFleetSessions    = runner.ListFleetSessions
	killAllFleetSessions = runner.KillAllFleetSessions
)

var (
	flagLast     bool
	flagKill     bool
	flagKillAll  bool
	flagStatus   bool
	flagDoctor   bool
	flagForce    bool
	flagRelayURL string
)

// resolveRelayURL is the single priority chain for relay URL resolution:
// --relay-url flag > config URL > built-in default.
func resolveRelayURL(flagURL, configURL string) string {
	if flagURL != "" {
		return flagURL
	}
	if configURL != "" {
		return configURL
	}
	return defaultRelayURL
}

func main() {
	root := &cobra.Command{
		Use:   "fleet",
		Short: "⚡ Launch multi-agent Claude Code fleets",
		RunE:  run,
	}

	root.Flags().BoolVar(&flagLast, "last", false, "Relaunch last saved config")
	root.Flags().BoolVar(&flagKill, "kill", false, "Stop fleet sessions for the last project")
	root.Flags().BoolVar(&flagKillAll, "kill-all", false, "Stop ALL fleet sessions across all projects")
	root.Flags().BoolVar(&flagStatus, "status", false, "List active fleet sessions")
	root.Flags().BoolVar(&flagDoctor, "doctor", false, "Check & install prerequisites")
	root.Flags().BoolVar(&flagForce, "force", false, "Skip the --kill-all confirmation prompt")
	root.PersistentFlags().StringVar(&flagRelayURL, "relay-url", "", "Override the relay URL for every command")

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

	usageCmd := &cobra.Command{
		Use:   "usage",
		Short: "Show per-project fleet usage (agents, polling, tasks, vault)",
		RunE:  runUsage,
	}
	root.AddCommand(usageCmd)

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
	case flagKillAll:
		return runKillAll()
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
	checks := doctor.Run(resolveRelayURL(flagRelayURL, ""))
	doctor.Print(checks)
	return nil
}

func runKill() error {
	cfg, err := loadLastConfig()
	if err != nil {
		return fmt.Errorf("no saved fleet config found for --kill; use --kill-all to stop every project's sessions")
	}

	// Auto-save before killing
	config.SaveAsLast(cfg)
	fmt.Printf("  Config saved for %s.\n", cfg.Project.Name)

	sessions, err := runner.ListProjectSessions(cfg.Project.Name)
	if err != nil {
		return fmt.Errorf("cannot list tmux sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Printf("  No fleet sessions running for project %q.\n", cfg.Project.Name)
		return nil
	}

	killed, _ := runner.KillProjectSessions(cfg.Project.Name)
	fmt.Printf("  Killed %d session(s) for project %q.\n", killed, cfg.Project.Name)

	remaining, _ := runner.ListFleetSessions()
	if len(remaining) > 0 {
		fmt.Printf("  Note: %d session(s) from other projects still running. Use --kill-all to stop all.\n", len(remaining))
	}
	return nil
}

func runKillAll() error {
	return killAll(os.Stdin, os.Stdout, flagForce)
}

// killAll stops every fleet session of every project. The blast radius is
// total, so without --force it requires an explicit y/N confirmation; EOF or
// anything but y/yes aborts.
func killAll(in io.Reader, out io.Writer, force bool) error {
	sessions, err := listFleetSessions()
	if err != nil {
		return fmt.Errorf("cannot list tmux sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(out, "  No fleet sessions running.")
		return nil
	}
	if !force {
		fmt.Fprintf(out, "  Kill %d fleet session(s) across ALL projects? [y/N] ", len(sessions))
		if !confirmYes(in) {
			fmt.Fprintln(out, "  Aborted. Use --force to skip this prompt.")
			return nil
		}
	}
	killAllFleetSessions()
	fmt.Fprintf(out, "  Killed %d fleet session(s) across all projects.\n", len(sessions))
	return nil
}

func confirmYes(in io.Reader) bool {
	line, _ := bufio.NewReader(in).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func runDispatch(cmd *cobra.Command, args []string) error {
	agent, _ := cmd.Flags().GetString("to")
	description := strings.Join(args, " ")

	cfg, err := config.LoadLast()
	if err != nil {
		return fmt.Errorf("no fleet config found. Run 'fleet' first")
	}

	client := relay.NewClient(resolveRelayURL(flagRelayURL, cfg.Project.RelayURL))
	if err := client.DispatchTask(agent, cfg.Project.Name, description); err != nil {
		return fmt.Errorf("dispatch failed: %w", err)
	}

	if err := runner.WakeAgent(cfg.Project.Name, agent); err != nil {
		return fmt.Errorf("wake failed: %w", err)
	}

	fmt.Printf("  ✓ Task dispatched to %s and agent woken\n", agent)
	return nil
}

func runLogs(cmd *cobra.Command, args []string) error {
	agent := args[0]
	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")
	if lines < 1 {
		return fmt.Errorf("--lines must be at least 1, got %d", lines)
	}

	cfg, err := config.LoadLast()
	if err != nil {
		return fmt.Errorf("no fleet config found. Run 'fleet' first")
	}
	project := cfg.Project.Name

	if !runner.HasSession(project, agent) {
		return fmt.Errorf("no fleet session for agent %q in project %q", agent, project)
	}

	output, err := runner.CapturePane(project, agent)
	if err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}

	if !follow {
		fmt.Println(tailLines(output, lines))
		return nil
	}

	return followPane(os.Stdout, func() (string, error) {
		return runner.CapturePane(project, agent)
	}, agent, logsHeader(project, agent), output, lines, 1*time.Second)
}

func tailLines(output string, n int) string {
	if n <= 0 {
		return ""
	}
	// capture-pane output ends with a newline; without this trim the trailing
	// empty element eats one of the n requested lines.
	allLines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(allLines) > n {
		allLines = allLines[len(allLines)-n:]
	}
	return strings.Join(allLines, "\n")
}

func logsHeader(project, agent string) string {
	return fmt.Sprintf("  ⚡ fleet logs — %s/%s (Ctrl-C to stop)\n\n", project, agent)
}

func followPane(w io.Writer, capture func() (string, error), agent, header, initial string, lines int, interval time.Duration) error {
	// Clear once so the initial frame and every \033[H refresh share the same
	// top-left origin instead of interleaving with the shell scrollback.
	fmt.Fprint(w, "\033[2J\033[H")
	writeFrame(w, header, tailLines(initial, lines))
	prev := initial
	for {
		time.Sleep(interval)
		current, err := capture()
		if err != nil {
			return fmt.Errorf("lost tmux session for agent %q: %w", agent, err)
		}
		if current != prev {
			fmt.Fprint(w, "\033[H")
			writeFrame(w, header, tailLines(current, lines))
			prev = current
		}
	}
}

// writeFrame draws header+frame, erasing each line's leftover tail (\033[K)
// and everything below the frame (\033[J) so a redraw shorter than the
// previous one leaves no stale characters on screen.
func writeFrame(w io.Writer, header, frame string) {
	fmt.Fprint(w, strings.ReplaceAll(header+frame, "\n", "\033[K\n"), "\033[J")
}

func runAdd(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	role, _ := cmd.Flags().GetString("role")
	color, _ := cmd.Flags().GetString("color")
	reportsTo, _ := cmd.Flags().GetString("reports-to")

	cfg, err := loadLastConfig()
	if err != nil {
		return fmt.Errorf("no fleet config found. Run 'fleet' first")
	}
	project := cfg.Project.Name

	agent := config.AgentConfig{
		Name:      name,
		Color:     color,
		Role:      role,
		ReportsTo: reportsTo,
	}

	// Validate the resulting fleet before creating any session — an invalid
	// name/role (P0-2) or a duplicate must not produce a half-broken agent.
	cfg.Agents = append(cfg.Agents, agent)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid agent: %w", err)
	}

	if runner.HasSession(project, name) {
		return fmt.Errorf("agent %q already has a running session", name)
	}

	claudeBin, err := cfg.Claude.ResolveBin()
	if err != nil {
		return err
	}
	claudeCmd := claudeBin
	for _, f := range cfg.Claude.Flags {
		claudeCmd += " " + f
	}

	if err := runner.CreateSession(project, name, cfg.Project.Cwd); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	if err := runner.SendKeys(project, name, claudeCmd); err != nil {
		return fmt.Errorf("failed to launch claude: %w", err)
	}

	// Register on the relay (profile + profile_slug) so the agent can actually
	// receive dispatched tasks — without this it's a ghost in the grid.
	client := relay.NewClient(resolveRelayURL(flagRelayURL, cfg.Project.RelayURL))
	if err := client.EnsureProfile(name, role, project); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ profile registration failed: %v\n", err)
	}
	if err := client.RegisterAgent(name, project, role, name); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ relay registration failed: %v\n", err)
	}

	config.SaveAsLast(cfg)

	fmt.Printf("  ✓ Agent %s added to %s.\n", name, project)
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	agent := args[0]

	cfg, err := config.LoadLast()
	if err != nil {
		return fmt.Errorf("no fleet config found. Run 'fleet' first")
	}
	project := cfg.Project.Name

	if !runner.HasSession(project, agent) {
		return fmt.Errorf("no fleet session for agent %q in project %q", agent, project)
	}

	runner.SendKeys(project, agent, "/exit")
	if !runner.WaitSessionGone(project, agent, 3*time.Second) {
		runner.KillSession(project, agent)
	}

	// Deregister from the relay so the agent doesn't linger as a ghost.
	if err := relay.NewClient(resolveRelayURL(flagRelayURL, cfg.Project.RelayURL)).DeactivateAgent(agent, project); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Agent stopped but relay deregistration failed: %v\n", err)
	}

	var updated []config.AgentConfig
	for _, a := range cfg.Agents {
		if a.Name != agent {
			updated = append(updated, a)
		}
	}
	cfg.Agents = updated
	config.SaveAsLast(cfg)

	sessions, _ := runner.ListProjectSessions(project)
	if len(sessions) == 0 {
		fmt.Printf("  ✓ Agent %s stopped. Fleet %s is now empty.\n", agent, project)
	} else {
		fmt.Printf("  ✓ Agent %s stopped. %d agents remaining in %s.\n", agent, len(sessions), project)
	}
	return nil
}

func runLast() error {
	cfg, err := config.LoadLast()
	if err != nil {
		fmt.Println("  No saved config found. Run 'fleet' to create one.")
		return runWizard()
	}

	cfg.Project.RelayURL = resolveRelayURL(flagRelayURL, cfg.Project.RelayURL)

	var agentNames []string
	for _, a := range cfg.Agents {
		agentNames = append(agentNames, a.Name)
	}
	existing := runner.DetectConflicts(cfg.Project.Name, agentNames)

	if len(existing) == len(cfg.Agents) {
		fmt.Printf("  Fleet %s is already running (%d agents).\n", cfg.Project.Name, len(existing))
		fmt.Println("  Use 'fleet --status' to check or 'fleet --kill' to stop.")
		return nil
	}

	if len(existing) > 0 {
		fmt.Printf("  ⚠ %d/%d agents already running in %s\n", len(existing), len(cfg.Agents), cfg.Project.Name)
	}

	fmt.Printf("  ⚡ Relaunching %s (%d agents)\n\n", cfg.Project.Name, len(cfg.Agents))
	return launch(cfg, false)
}

func runWizard() error {
	client := relay.NewClient(resolveRelayURL(flagRelayURL, ""))

	result, err := wizard.Run(client)
	if err != nil {
		if err.Error() == "cancelled" {
			return nil
		}
		return err
	}

	result.Config.Project.RelayURL = resolveRelayURL(flagRelayURL, result.Config.Project.RelayURL)

	return launch(result.Config, result.Save)
}

// reportLaunchResults writes each failed agent's error to w and returns a
// non-nil error if any agent failed to launch. Without this a partial launch
// was reported as success with exit code 0, hiding orphaned/missing sessions.
func reportLaunchResults(w io.Writer, results []runner.LaunchResult) error {
	failed := 0
	for _, r := range results {
		if !r.Success {
			failed++
			fmt.Fprintf(w, "  ✗ %s failed: %v\n", r.Agent, r.Error)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d agent(s) failed to launch", failed, len(results))
	}
	return nil
}

func launch(cfg *config.FleetConfig, save bool) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	claudeBin, err := cfg.Claude.ResolveBin()
	if err != nil {
		fmt.Printf("  ✗ %v\n", err)
		fmt.Println("    Run 'fleet --doctor' to check prerequisites.")
		return err
	}

	relayClient := relay.NewClient(cfg.Project.RelayURL)
	if err := relayClient.Health(); err != nil {
		fmt.Printf("  ✗ Relay unreachable at %s\n", cfg.Project.RelayURL)
		fmt.Println("    Run 'fleet --doctor' to check prerequisites.")
		return fmt.Errorf("relay health check failed: %w", err)
	}

	// Preflight: fail early on a broken/absent tmux rather than per-agent later.
	if _, err := runner.ListFleetSessions(); err != nil {
		fmt.Printf("  ✗ %v\n", err)
		fmt.Println("    Run 'fleet --doctor' to check prerequisites.")
		return err
	}

	// Always save config
	if err := config.SaveAsLast(cfg); err != nil {
		fmt.Printf("  ⚠ Failed to save config: %v\n", err)
	} else if save {
		fmt.Println("  Config saved to ~/.fleet/configs/" + cfg.Project.Name + ".toml")
	}

	fmt.Print("\n  🚀 Launching fleet...\n\n")

	// Phase 1: Create tmux sessions + launch claude (fast)
	results := runner.CreateSessions(cfg, claudeBin)

	launchErr := reportLaunchResults(os.Stderr, results)

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
	runner.OpenITerm2Grid(cfg.Project.Name, agentNames)

	// Phase 3: Configure agents in background via a shell script
	// (fleet exits, script waits for prompts and sends init commands)
	logPath, cfgErr := runner.ConfigureAgentsAsync(cfg)
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Agent configuration failed to start: %v\n", cfgErr)
		if launchErr == nil {
			launchErr = cfgErr
		}
	}

	if launchErr != nil {
		fmt.Printf("\n  ⚠ Fleet partially launched: %d/%d sessions created.\n", success, len(cfg.Agents))
	} else {
		fmt.Printf("\n  ✅ Fleet launched. %d/%d sessions created.\n", success, len(cfg.Agents))
	}
	fmt.Print("  Agents configuring in background (watch iTerm2 panes).\n")
	if logPath != "" {
		fmt.Printf("  Config log: %s\n", logPath)
	}
	fmt.Println()
	return launchErr
}
