package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/cost"
	"github.com/zairedegrees/fleet/internal/term"
)

// agentCost is one agent's measured spend. A non-empty Note means the value is
// unknown ("?") — never rendered as $0.
type agentCost struct {
	Name     string
	ByModel  map[string]cost.Usage // measured, per model
	USD      float64               // summed across priced models
	USDKnown bool                  // false if any model lacked a rate
	Note     string                // why a value is unknown
}

// projectCost is the per-project view rendered by `fleet cost`.
type projectCost struct {
	Project      string
	RelayURL     string
	Window       string // human label, e.g. "since today (00:00 local)"
	Agents       []agentCost
	TotalUSD     float64
	TotalKnown   bool
	RelayWarning string // relay unreachable → all agents unknown for this project
}

// renderCost is pure (data in → string out) so the display logic is testable
// without configs, a relay, or a filesystem.
func renderCost(projects []projectCost) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %d project(s):\n\n", len(projects))
	for _, p := range projects {
		fmt.Fprintf(&b, "    [%s]  relay: %s   window: %s\n",
			term.Sanitize(p.Project), term.Sanitize(p.RelayURL), term.Sanitize(p.Window))
		if p.RelayWarning != "" {
			fmt.Fprintf(&b, "      live (relay): ?  ⚠ %s\n\n", term.Sanitize(p.RelayWarning))
			continue
		}
		for _, a := range p.Agents {
			if a.Note != "" {
				fmt.Fprintf(&b, "      %-10s %s  →  ?\n", term.Sanitize(a.Name), term.Sanitize(a.Note))
				continue
			}
			fmt.Fprintf(&b, "      %-10s %s  →  %s   [measured]\n",
				term.Sanitize(a.Name), modelTokenSummary(a.ByModel), usdLabel(a.USD, a.USDKnown))
		}
		b.WriteString("      ─────\n")
		fmt.Fprintf(&b, "      %-10s →  %s\n", "total", usdLabel(p.TotalUSD, p.TotalKnown))
		b.WriteString("\n")
	}
	return b.String()
}

func usdLabel(usd float64, known bool) string {
	if !known {
		return "$?"
	}
	return fmt.Sprintf("$%.2f", usd)
}

func compactTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func shortModel(m string) string {
	return strings.TrimPrefix(m, "claude-")
}

// parseSince turns the --since flag into a lower bound and a human label.
// "" / "today" → local midnight; "all" → zero time (no bound); a Go duration
// → now-duration. now is a parameter so the resolution stays testable.
func parseSince(s string, now time.Time) (time.Time, string, error) {
	switch s {
	case "", "today":
		y, mo, d := now.Date()
		return time.Date(y, mo, d, 0, 0, 0, 0, now.Location()), "since today (00:00 local)", nil
	case "all":
		return time.Time{}, "all (entire session)", nil
	default:
		dur, err := time.ParseDuration(s)
		if err != nil {
			return time.Time{}, "", fmt.Errorf("invalid --since %q: use 'today', 'all', or a duration like 24h", s)
		}
		return now.Add(-dur), fmt.Sprintf("since %s ago", s), nil
	}
}

// modelTokenSummary renders one segment per model, models sorted for stable output.
func modelTokenSummary(byModel map[string]cost.Usage) string {
	if len(byModel) == 0 {
		return "in 0 · out 0 · cache 0"
	}
	models := make([]string, 0, len(byModel))
	for m := range byModel {
		models = append(models, m)
	}
	sort.Strings(models)
	parts := make([]string, 0, len(models))
	for _, m := range models {
		u := byModel[m]
		parts = append(parts, fmt.Sprintf("%s in %s · out %s · cache-r %s · cache-w %s",
			term.Sanitize(shortModel(m)), compactTokens(u.In), compactTokens(u.Out),
			compactTokens(u.CacheRead), compactTokens(u.CacheCreate)))
	}
	return strings.Join(parts, "  |  ")
}

var (
	flagCostSince   string
	flagCostProject string
)

func runCost(cmd *cobra.Command, args []string) error {
	configs := loadSavedConfigs()
	if len(configs) == 0 {
		fmt.Println("  No saved fleet configs in ~/.fleet/configs — nothing to report.")
		return nil
	}
	if flagCostProject != "" {
		filtered := configs[:0]
		for _, c := range configs {
			if strings.EqualFold(c.Project.Name, flagCostProject) {
				filtered = append(filtered, c)
			}
		}
		configs = filtered
		if len(configs) == 0 {
			fmt.Printf("  No saved config for project %q.\n", flagCostProject)
			return nil
		}
	}

	since, label, err := parseSince(flagCostSince, time.Now())
	if err != nil {
		return err
	}

	fallback := defaultRelayURL
	if cfg, err := loadLastConfig(); err == nil && cfg.Project.RelayURL != "" {
		fallback = cfg.Project.RelayURL
	}

	fmt.Print(renderCost(buildCost(configs, flagRelayURL, fallback, since, label, cost.DefaultProjectsDir())))
	return nil
}

// buildCost turns saved configs into the measured per-project cost view. Relay
// URLs resolve like --status/--usage. Every unknown (no session_id, no
// transcript, unpriced model, relay down) renders "?" — never a faked 0; a
// project total is unknown if any contributing agent's USD is unknown.
func buildCost(configs []*config.FleetConfig, override, fallback string, since time.Time, windowLabel, projectsDir string) []projectCost {
	clients := map[string]relayQuerier{}
	relayDown := map[string]string{}
	var out []projectCost

	for _, cfg := range configs {
		pc := projectCost{Project: cfg.Project.Name, Window: windowLabel, TotalKnown: true}

		url := override
		if url == "" {
			url = relayURLFor(cfg.Project.Name, configs, fallback)
		}
		pc.RelayURL = url

		client, ok := clients[url]
		if !ok {
			client = newStatusClient(url)
			clients[url] = client
		}

		if reason, down := relayDown[url]; down {
			pc.RelayWarning = reason
			pc.TotalKnown = false
			out = append(out, pc)
			continue
		}
		agents, err := client.ListAgents(cfg.Project.Name)
		if err != nil {
			pc.RelayWarning = fmt.Sprintf("relay unavailable at %s (%v)", url, err)
			relayDown[url] = pc.RelayWarning
			pc.TotalKnown = false
			out = append(out, pc)
			continue
		}

		for _, a := range agents {
			ac := agentCost{Name: a.Name}
			switch {
			case a.SessionID == "":
				ac.Note = "(no session_id — agent never ran)"
			default:
				path, err := cost.ResolveTranscript(projectsDir, a.SessionID)
				if err != nil {
					ac.Note = "(no transcript yet)"
					break
				}
				byModel, err := cost.ScanTranscript(path, since)
				if err != nil {
					ac.Note = "(transcript unreadable)"
					break
				}
				ac.ByModel = byModel
				ac.USDKnown = true
				for model, u := range byModel {
					usd, known := cost.CostUSD(u, model)
					if !known {
						ac.USDKnown = false
						continue
					}
					ac.USD += usd
				}
			}

			if ac.Note != "" || !ac.USDKnown {
				pc.TotalKnown = false
			} else {
				pc.TotalUSD += ac.USD
			}
			pc.Agents = append(pc.Agents, ac)
		}
		out = append(out, pc)
	}
	return out
}

func newCostCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "cost",
		Short: "Measured token spend per agent, from Claude Code transcripts",
		RunE:  runCost,
	}
	c.Flags().StringVar(&flagCostSince, "since", "today", "Window: today | all | a Go duration like 24h")
	c.Flags().StringVar(&flagCostProject, "project", "", "Limit to one project by name")
	return c
}
