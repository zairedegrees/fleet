package main

import (
	"fmt"
	"sort"
	"strings"

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
				fmt.Fprintf(&b, "      %-10s %s  →  ?\n", term.Sanitize(a.Name), a.Note)
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
			shortModel(m), compactTokens(u.In), compactTokens(u.Out),
			compactTokens(u.CacheRead), compactTokens(u.CacheCreate)))
	}
	return strings.Join(parts, "  |  ")
}
