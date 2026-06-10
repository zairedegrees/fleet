package main

import (
	"fmt"
	"strings"

	"github.com/zairedegrees/fleet/internal/relay"
	"github.com/zairedegrees/fleet/internal/runner"
)

// agentStatus is the per-agent view rendered by `fleet --status`: relay
// registration state + task count from the relay (the source of truth), and
// tmux session existence as the liveness signal. Tasks -1 means unknown —
// never faked as 0.
type agentStatus struct {
	Session    string
	Agent      string
	RelayState string // relay status, "unregistered", or "" when relay is down
	Tasks      int
	HasSession bool
}

type projectStatus struct {
	Project string
	Agents  []agentStatus
}

func runStatus() error {
	sessions, err := runner.ListFleetSessions()
	if err != nil {
		return fmt.Errorf("cannot list tmux sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("  No fleet sessions running.")
		return nil
	}

	// Group sessions by project for display
	grouped := make(map[string][]string)
	var order []string
	for _, s := range sessions {
		project := extractProject(s)
		if _, seen := grouped[project]; !seen {
			order = append(order, project)
		}
		grouped[project] = append(grouped[project], s)
	}

	relayURL := defaultRelayURL
	if cfg, err := loadLastConfig(); err == nil && cfg.Project.RelayURL != "" {
		relayURL = cfg.Project.RelayURL
	}
	client := relay.NewClient(relayURL)

	relayUp := true
	relayWarning := ""
	var projects []projectStatus
	for _, project := range order {
		var relayAgents []relay.Agent
		if relayUp {
			agents, err := client.ListAgents(project)
			if err != nil {
				relayUp = false
				relayWarning = fmt.Sprintf("relay unavailable at %s — showing tmux sessions only (%v)", relayURL, err)
			} else {
				relayAgents = agents
			}
		}
		tasks := make(map[string]int)
		if relayUp {
			for _, a := range relayAgents {
				profile := a.ProfileSlug
				if profile == "" {
					profile = a.Name
				}
				if n, err := client.CountActiveTasks(project, profile); err == nil {
					tasks[a.Name] = n
				}
			}
		}
		projects = append(projects, projectStatus{
			Project: project,
			Agents:  mergeAgents(project, grouped[project], relayAgents, tasks, relayUp),
		})
	}

	fmt.Print(renderStatus(projects, len(sessions), relayWarning))
	return nil
}

// mergeAgents unions the tmux sessions of a project with its relay-registered
// agents: sessions keep tmux as the liveness signal, the relay provides state
// and workload, and relay-only agents surface as ghosts without a session.
func mergeAgents(project string, sessions []string, relayAgents []relay.Agent, tasks map[string]int, relayUp bool) []agentStatus {
	byName := make(map[string]relay.Agent, len(relayAgents))
	for _, a := range relayAgents {
		byName[a.Name] = a
	}

	var out []agentStatus
	seen := make(map[string]bool)
	for _, s := range sessions {
		name := runner.AgentFromSession(project, s)
		seen[name] = true
		st := agentStatus{Session: s, Agent: name, Tasks: -1, HasSession: true}
		if relayUp {
			if a, ok := byName[name]; ok {
				st.RelayState = a.Status
				if n, ok := tasks[name]; ok {
					st.Tasks = n
				}
			} else {
				st.RelayState = "unregistered"
			}
		}
		out = append(out, st)
	}
	for _, a := range relayAgents {
		if seen[a.Name] {
			continue
		}
		st := agentStatus{Agent: a.Name, RelayState: a.Status, Tasks: -1}
		if n, ok := tasks[a.Name]; ok {
			st.Tasks = n
		}
		out = append(out, st)
	}
	return out
}

// renderStatus is pure (data in → string out) so the display logic is testable
// without tmux or a relay.
func renderStatus(projects []projectStatus, sessionCount int, relayWarning string) string {
	var b strings.Builder
	if relayWarning != "" {
		fmt.Fprintf(&b, "  ⚠ %s\n\n", relayWarning)
	}
	fmt.Fprintf(&b, "  %d fleet session(s):\n\n", sessionCount)
	for _, p := range projects {
		fmt.Fprintf(&b, "    [%s]\n", p.Project)
		for _, a := range p.Agents {
			fmt.Fprintf(&b, "      %s\n", agentLine(a))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func agentLine(a agentStatus) string {
	label := a.Session
	if label == "" {
		label = a.Agent
	}
	var parts []string
	if a.RelayState != "" {
		parts = append(parts, "relay: "+a.RelayState)
		if a.RelayState != "unregistered" {
			if a.Tasks >= 0 {
				parts = append(parts, fmt.Sprintf("%d task(s)", a.Tasks))
			} else {
				parts = append(parts, "tasks: ?")
			}
		}
	}
	if !a.HasSession {
		parts = append(parts, "no tmux session")
	}
	if len(parts) == 0 {
		return label
	}
	return label + "  [" + strings.Join(parts, " · ") + "]"
}

// extractProject parses the project name from a fleet session name.
// Format: "fleet-{project}-{agent}"
// We use the last config to identify known project names, otherwise
// we take everything between "fleet-" and the last "-".
func extractProject(session string) string {
	// Strip "fleet-" prefix
	rest := session[len("fleet-"):]
	// The agent name is after the last dash
	lastDash := lastIndexByte(rest, '-')
	if lastDash < 0 {
		return rest
	}
	return rest[:lastDash]
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
