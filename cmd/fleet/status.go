package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
	"github.com/zairedegrees/fleet/internal/runner"
)

// relayStateUnknown marks a session that matched no known project name: its
// real project — and therefore its relay registration — is unknown, so status
// says "?" instead of asserting "unregistered".
const relayStateUnknown = "?"

// statusRelayTimeout keeps `fleet --status` snappy: a hanging relay must not
// stall the listing for the default client's 10s.
const statusRelayTimeout = 2 * time.Second

// agentStatus is the per-agent view rendered by `fleet --status`: relay
// registration state + task count from the relay (the source of truth), and
// tmux session existence as the liveness signal. Tasks -1 means unknown —
// never faked as 0.
type agentStatus struct {
	Session    string
	Agent      string
	RelayState string // relay status, "unregistered", "?" (unknown project), or "" when relay is down
	Tasks      int
	HasSession bool
}

type projectStatus struct {
	Project string
	Agents  []agentStatus
}

// relayQuerier is the slice of relay.Client that status needs — a seam so the
// status pipeline is testable without a relay.
type relayQuerier interface {
	ListAgents(project string) ([]relay.Agent, error)
	CountActiveTasks(project, profile string) (int, error)
}

var newStatusClient = func(url string) relayQuerier {
	return relay.NewClientWithTimeout(url, statusRelayTimeout)
}

// loadSavedConfigs reads every saved project config — they are what lets
// status resolve session names against real project names. Falls back to the
// last config when the configs dir is empty.
var loadSavedConfigs = func() []*config.FleetConfig {
	paths, _ := filepath.Glob(filepath.Join(config.FleetDir(), "configs", "*.toml"))
	var cfgs []*config.FleetConfig
	for _, p := range paths {
		if cfg, err := config.Load(p); err == nil && cfg.Project.Name != "" {
			cfgs = append(cfgs, cfg)
		}
	}
	if len(cfgs) == 0 {
		if cfg, err := loadLastConfig(); err == nil && cfg.Project.Name != "" {
			cfgs = append(cfgs, cfg)
		}
	}
	return cfgs
}

func runStatus() error {
	sessions, err := runner.ListFleetSessions()
	if err != nil {
		return fmt.Errorf("cannot list tmux sessions: %w", err)
	}

	defaultURL := defaultRelayURL
	if cfg, err := loadLastConfig(); err == nil && cfg.Project.RelayURL != "" {
		defaultURL = cfg.Project.RelayURL
	}

	projects, warning := buildStatus(sessions, loadSavedConfigs(), defaultURL)
	if len(sessions) == 0 && len(projects) == 0 {
		if warning != "" {
			fmt.Printf("  ⚠ %s\n\n", warning)
		}
		fmt.Println("  No fleet sessions running.")
		return nil
	}
	fmt.Print(renderStatus(projects, len(sessions), warning))
	return nil
}

// buildStatus turns the tmux session list + saved configs into the per-project
// status view. Sessions resolve against KNOWN project names so dash-named
// agents and dot-projects group under the real project the relay was
// registered with; sessions matching no known project render an honest "?".
// Known projects are queried on their own relay URL even with zero sessions,
// so relay-only ghosts stay visible.
func buildStatus(sessions []string, configs []*config.FleetConfig, defaultURL string) ([]projectStatus, string) {
	var knownNames []string
	for _, c := range configs {
		knownNames = append(knownNames, c.Project.Name)
	}

	grouped := make(map[string][]string)
	knownGroup := make(map[string]bool)
	var order []string
	for _, s := range sessions {
		project, _, known := resolveSession(s, knownNames)
		if _, seen := grouped[project]; !seen {
			order = append(order, project)
			knownGroup[project] = known
		}
		grouped[project] = append(grouped[project], s)
	}
	for _, name := range knownNames {
		if _, seen := grouped[name]; !seen {
			grouped[name] = nil
			knownGroup[name] = true
			order = append(order, name)
		}
	}

	clients := make(map[string]relayQuerier)
	relayDown := make(map[string]bool)
	var warnings []string
	var projects []projectStatus
	for _, project := range order {
		if !knownGroup[project] {
			var agents []agentStatus
			for _, s := range grouped[project] {
				_, agent, _ := resolveSession(s, knownNames)
				agents = append(agents, agentStatus{Session: s, Agent: agent, RelayState: relayStateUnknown, Tasks: -1, HasSession: true})
			}
			projects = append(projects, projectStatus{Project: project, Agents: agents})
			continue
		}

		url := relayURLFor(project, configs, defaultURL)
		client, ok := clients[url]
		if !ok {
			client = newStatusClient(url)
			clients[url] = client
		}

		relayUp := !relayDown[url]
		var relayAgents []relay.Agent
		if relayUp {
			agents, err := client.ListAgents(project)
			if err != nil {
				relayDown[url] = true
				relayUp = false
				warnings = append(warnings, fmt.Sprintf("relay unavailable at %s — showing tmux sessions only (%v)", url, err))
			} else {
				relayAgents = agents
			}
		}
		var tasks map[string]int
		if relayUp {
			tasks = fetchTaskCounts(client, project, relayAgents)
		}
		merged := mergeAgents(project, grouped[project], relayAgents, tasks, relayUp)
		if len(merged) == 0 {
			continue
		}
		projects = append(projects, projectStatus{Project: project, Agents: merged})
	}

	return projects, strings.Join(warnings, "; ")
}

// relayURLFor resolves a project's relay URL from its own saved config,
// falling back to the given default — one project's relay must not answer for
// another's.
func relayURLFor(project string, configs []*config.FleetConfig, fallback string) string {
	for _, c := range configs {
		if c.Project.Name == project && c.Project.RelayURL != "" {
			return c.Project.RelayURL
		}
	}
	return fallback
}

// fetchTaskCounts queries the relay once per unique profile slug and fans the
// count out to every agent sharing it — shared slugs must not double-query.
// Failed fetches leave no entry, so the agent renders an honest "tasks: ?".
func fetchTaskCounts(client relayQuerier, project string, agents []relay.Agent) map[string]int {
	byProfile := make(map[string]int)
	attempted := make(map[string]bool)
	counts := make(map[string]int)
	for _, a := range agents {
		profile := a.ProfileSlug
		if profile == "" {
			profile = a.Name
		}
		if !attempted[profile] {
			attempted[profile] = true
			if n, err := client.CountActiveTasks(project, profile); err == nil {
				byProfile[profile] = n
			}
		}
		if n, ok := byProfile[profile]; ok {
			counts[a.Name] = n
		}
	}
	return counts
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
		if a.RelayState != "unregistered" && a.RelayState != relayStateUnknown {
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

// resolveSession matches a fleet session against the known project names: the
// longest "fleet-<sanitizedProject>-" prefix wins, so dash-named agents
// (ux-designer) and dot-projects (v1stud.io → v1stud-io in tmux) resolve to
// the REAL project name the relay was registered with. Sessions matching no
// known project fall back to the last-dash guess with known=false.
func resolveSession(session string, projects []string) (project, agent string, known bool) {
	bestLen := 0
	for _, p := range projects {
		prefix := runner.SessionName(p, "")
		if len(session) > len(prefix) && strings.HasPrefix(session, prefix) && len(prefix) > bestLen {
			bestLen = len(prefix)
			project = p
		}
	}
	if bestLen > 0 {
		return project, session[bestLen:], true
	}
	return extractProject(session), guessAgent(session), false
}

// extractProject guesses the project name from a fleet session name
// ("fleet-{project}-{agent}") by splitting on the last dash. Only a fallback
// label for sessions that match no known project — the guess is ambiguous for
// dash-named agents, which is why such sessions render "relay: ?".
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

// guessAgent is extractProject's counterpart: the part after the last dash.
func guessAgent(session string) string {
	rest := session[len("fleet-"):]
	lastDash := lastIndexByte(rest, '-')
	if lastDash < 0 {
		return rest
	}
	return rest[lastDash+1:]
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
