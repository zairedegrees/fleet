package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
	"github.com/zairedegrees/fleet/internal/runner"
	"github.com/zairedegrees/fleet/internal/term"
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
	AutoTalk   bool   // config posture: greets at boot vs woken on demand
	LastSeen   string // relay last_seen (RFC3339), "" when unknown
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

// statusOnce gathers the current fleet status and returns it as a ready-to-print
// string. Extracted from runStatus so both the one-shot and --watch paths share
// exactly one rendering path. The error is only the unrecoverable tmux failure;
// a down relay degrades inside buildStatus (a warning line), it does not error.
func statusOnce() (string, error) {
	sessions, err := runner.ListFleetSessions()
	if err != nil {
		return "", fmt.Errorf("cannot list tmux sessions: %w", err)
	}

	defaultURL := defaultRelayURL
	if cfg, err := loadLastConfig(); err == nil && cfg.Project.RelayURL != "" {
		defaultURL = cfg.Project.RelayURL
	}

	projects, warning := buildStatus(sessions, loadSavedConfigs(), defaultURL, flagRelayURL)
	if len(sessions) == 0 && len(projects) == 0 {
		s := ""
		if warning != "" {
			s += fmt.Sprintf("  ⚠ %s\n\n", warning)
		}
		return s + "  No fleet sessions running.\n", nil
	}
	return renderStatus(projects, len(sessions), warning, time.Now()), nil
}

func runStatus() error {
	out, err := statusOnce()
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// buildStatus turns the tmux session list + saved configs into the per-project
// status view. Sessions resolve against KNOWN project names so dash-named
// agents and dot-projects group under the real project the relay was
// registered with; sessions matching no known project render an honest "?".
// Known projects are queried on their own relay URL even with zero sessions,
// so relay-only ghosts stay visible. A non-empty override (--relay-url) beats
// every per-project resolution.
func buildStatus(sessions []string, configs []*config.FleetConfig, defaultURL, override string) ([]projectStatus, string) {
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

		url := override
		if url == "" {
			url = relayURLFor(project, configs, defaultURL)
		}
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
		merged := mergeAgents(project, grouped[project], relayAgents, tasks, postureFor(project, configs), relayUp)
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
// agents: sessions keep tmux as the liveness signal, the relay provides state,
// workload and last_seen, and relay-only agents surface as ghosts. posture maps
// agent name → auto_talk from the saved config, so status can label posture.
func mergeAgents(project string, sessions []string, relayAgents []relay.Agent, tasks map[string]int, posture map[string]bool, relayUp bool) []agentStatus {
	byName := make(map[string]relay.Agent, len(relayAgents))
	for _, a := range relayAgents {
		byName[a.Name] = a
	}

	var out []agentStatus
	seen := make(map[string]bool)
	for _, s := range sessions {
		name := runner.AgentFromSession(project, s)
		seen[name] = true
		st := agentStatus{Session: s, Agent: name, Tasks: -1, HasSession: true, AutoTalk: posture[name]}
		if relayUp {
			if a, ok := byName[name]; ok {
				st.RelayState = a.Status
				st.LastSeen = a.LastSeen
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
		st := agentStatus{Agent: a.Name, RelayState: a.Status, LastSeen: a.LastSeen, Tasks: -1, AutoTalk: posture[a.Name]}
		if n, ok := tasks[a.Name]; ok {
			st.Tasks = n
		}
		out = append(out, st)
	}
	return out
}

// postureFor maps each agent name in a project's saved config to its auto_talk
// posture, so status can label agents auto-talk vs on-demand. Returns nil when
// the project has no saved config (posture then defaults to on-demand/false).
func postureFor(project string, configs []*config.FleetConfig) map[string]bool {
	for _, c := range configs {
		if c.Project.Name == project {
			m := make(map[string]bool, len(c.Agents))
			for _, a := range c.Agents {
				m[a.Name] = a.AutoTalk
			}
			return m
		}
	}
	return nil
}

// renderStatus is pure (data in → string out) so the display logic is testable
// without tmux or a relay. now drives the "seen Xm ago" segments.
func renderStatus(projects []projectStatus, sessionCount int, relayWarning string, now time.Time) string {
	var b strings.Builder
	if relayWarning != "" {
		fmt.Fprintf(&b, "  ⚠ %s\n\n", term.Sanitize(relayWarning))
	}
	fmt.Fprintf(&b, "  %d fleet session(s):\n\n", sessionCount)
	for _, p := range projects {
		fmt.Fprintf(&b, "    [%s]\n", term.Sanitize(p.Project))
		for _, a := range p.Agents {
			fmt.Fprintf(&b, "      %s\n", agentLine(a, now))
		}
		b.WriteString("\n")
	}
	if hasIdle(projects) {
		b.WriteString("  idle = registered, in standby (token discipline). Wake: fleet dispatch --to <agent> \"<task>\"\n")
	}
	return b.String()
}

// hasIdle reports whether any agent derives to the idle state, so the legend
// only appears when it explains something on screen.
func hasIdle(projects []projectStatus) bool {
	for _, p := range projects {
		for _, a := range p.Agents {
			if deriveOpState(a.RelayState, a.Tasks) == "idle" {
				return true
			}
		}
	}
	return false
}

// deriveOpState turns the relay's registration state + task count into an
// operator-facing word. "active" is a registration flag, not a liveness signal:
// an active agent with zero tasks is in standby, which we surface as "idle". An
// unknown task count (-1) must never render as idle — it becomes "registered"
// and the task segment shows "tasks: ?".
func deriveOpState(relayState string, tasks int) string {
	if relayState != "active" {
		return relayState
	}
	switch {
	case tasks < 0:
		return "registered"
	case tasks == 0:
		return "idle"
	default:
		return "working"
	}
}

// relativeTime renders an RFC3339 timestamp (the format coord writes last_seen
// in) as a compact "Xs/Xm/Xh/Xd ago" relative to now. Empty or unparsable input
// yields "" so the caller omits the segment; a future time (clock skew) clamps
// to "just now".
func relativeTime(lastSeen string, now time.Time) string {
	if lastSeen == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, lastSeen)
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func agentLine(a agentStatus, now time.Time) string {
	label := term.Sanitize(a.Agent)
	var parts []string
	if a.RelayState != "" {
		parts = append(parts, deriveOpState(a.RelayState, a.Tasks))
		// Posture is known only for registered ("active") agents.
		if a.RelayState == "active" {
			if a.AutoTalk {
				parts = append(parts, "auto-talk")
			} else {
				parts = append(parts, "on-demand")
			}
		}
		// Task detail: ">=1" shows the count (idle/0 is already conveyed by the
		// state word); a registered agent with an unknown count is honest "?".
		if a.RelayState != "unregistered" && a.RelayState != relayStateUnknown {
			if a.Tasks >= 1 {
				parts = append(parts, fmt.Sprintf("%d task(s)", a.Tasks))
			} else if a.Tasks < 0 {
				parts = append(parts, "tasks: ?")
			}
		}
		if seen := relativeTime(a.LastSeen, now); seen != "" {
			parts = append(parts, "seen "+seen)
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
// (ux-designer) and dot-projects (acme.io → acme-io in tmux) resolve to
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
