package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
	"github.com/zairedegrees/fleet/internal/term"
)

// projectUsage is the per-project view rendered by `fleet usage`. Every number
// carries its source: Agents/Polling and the vault figures are config-declared,
// Registered/Active/Tasks come from the relay. -1 means unknown — never faked
// as 0.
type projectUsage struct {
	Project  string
	RelayURL string

	Agents  int // config-declared
	Polling int // config-declared posture==always (greets at boot)

	Bounded        int     // config-declared posture==bounded
	ProjBoundedUSD float64 // projected daily $ ceiling for bounded agents (estimate)

	Registered   int    // relay, -1 unknown
	Active       int    // relay, -1 unknown
	Tasks        int    // relay, -1 unknown
	RelayWarning string // why live state is unknown, when it is

	VaultBytes int64  // config vault dir, -1 unknown
	VaultDocs  int    // -1 unknown
	VaultNote  string // why vault is unknown, when it is
}

func runUsage(cmd *cobra.Command, args []string) error {
	configs := loadSavedConfigs()
	if len(configs) == 0 {
		fmt.Println("  No saved fleet configs in ~/.fleet/configs — nothing to report.")
		return nil
	}

	fallback := defaultRelayURL
	if cfg, err := loadLastConfig(); err == nil && cfg.Project.RelayURL != "" {
		fallback = cfg.Project.RelayURL
	}

	fmt.Print(renderUsage(buildUsage(configs, flagRelayURL, fallback)))
	return nil
}

// buildUsage turns the saved configs into the per-project usage view. Relay
// URLs resolve like --status: a non-empty override (--relay-url) beats the
// project's own relay_url, which beats the fallback. A relay failure marks the
// live numbers unknown for that project with an explicit warning.
func buildUsage(configs []*config.FleetConfig, override, fallback string) []projectUsage {
	clients := make(map[string]relayQuerier)
	relayDown := make(map[string]string)
	var out []projectUsage
	for _, cfg := range configs {
		u := projectUsage{
			Project:    cfg.Project.Name,
			Agents:     len(cfg.Agents),
			Registered: -1,
			Active:     -1,
			Tasks:      -1,
		}
		for _, a := range cfg.Agents {
			if a.AutoTalk {
				u.Polling++
			}
			if a.IsBounded() {
				u.Bounded++
				p := cfg.ResolveBounded(a)
				u.ProjBoundedUSD += float64(p.MaxWakesPerDay) * p.CostPerWakeUSD
			}
		}

		url := override
		if url == "" {
			url = relayURLFor(cfg.Project.Name, configs, fallback)
		}
		u.RelayURL = url

		client, ok := clients[url]
		if !ok {
			client = newStatusClient(url)
			clients[url] = client
		}

		if reason, down := relayDown[url]; down {
			u.RelayWarning = reason
		} else if agents, err := client.ListAgents(cfg.Project.Name); err != nil {
			u.RelayWarning = fmt.Sprintf("relay unavailable at %s (%v)", url, err)
			relayDown[url] = u.RelayWarning
		} else {
			u.Registered = len(agents)
			u.Active = 0
			for _, a := range agents {
				if a.Status == "active" {
					u.Active++
				}
			}
			u.Tasks = sumActiveTasks(client, cfg.Project.Name, agents)
		}

		u.VaultBytes, u.VaultDocs, u.VaultNote = vaultUsage(cfg)
		out = append(out, u)
	}
	return out
}

// sumActiveTasks totals active tasks once per unique profile slug. Any failed
// fetch makes the whole total unknown (-1) — a partial sum would understate.
func sumActiveTasks(client relayQuerier, project string, agents []relay.Agent) int {
	seen := make(map[string]bool)
	total := 0
	for _, a := range agents {
		profile := a.ProfileSlug
		if profile == "" {
			profile = a.Name
		}
		if seen[profile] {
			continue
		}
		seen[profile] = true
		n, err := client.CountActiveTasks(project, profile)
		if err != nil {
			return -1
		}
		total += n
	}
	return total
}

// vaultUsage sums the vault doc bytes a launch would inject for this config:
// each agent's resolved docs, exactly as the configure script pushes them (a
// shared doc counts once per agent). A missing vault dir is a true 0; an
// unknowable one (no cwd, unreadable dir) is -1 with the reason.
func vaultUsage(cfg *config.FleetConfig) (bytes int64, docs int, note string) {
	if cfg.Project.Cwd == "" {
		return -1, -1, "project cwd not set in config — vault dir unknown"
	}
	vaultDir := filepath.Join(cfg.Project.Cwd, ".fleet", "vault")
	for _, agent := range cfg.Agents {
		resolved, err := config.ResolveVaultDocs(vaultDir, agent)
		if err != nil {
			return -1, -1, fmt.Sprintf("vault dir unreadable at %s (%v)", vaultDir, err)
		}
		docs += len(resolved)
		bytes += config.VaultSize(resolved)
	}
	return bytes, docs, ""
}

// renderUsage is pure (data in → string out) so the display logic is testable
// without configs, a relay, or a filesystem.
func renderUsage(projects []projectUsage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %d project(s):\n\n", len(projects))
	for _, p := range projects {
		fmt.Fprintf(&b, "    [%s]  relay: %s\n", term.Sanitize(p.Project), term.Sanitize(p.RelayURL))
		fmt.Fprintf(&b, "      agents (config): %d declared — %d polling (auto_talk), %d idle  [config]\n",
			p.Agents, p.Polling, p.Agents-p.Polling)
		if p.Bounded > 0 {
			fmt.Fprintf(&b, "      bounded (est):   %d agent(s) — ≤ ~$%.2f/day projected  [estimate]\n",
				p.Bounded, p.ProjBoundedUSD)
		}
		// Keyed on the -1 sentinel, not the warning text — an unknown count
		// must render "?" even if the reason got lost on the way.
		if p.Registered < 0 || p.Active < 0 {
			b.WriteString("      live (relay):    ?")
			if p.RelayWarning != "" {
				fmt.Fprintf(&b, "  ⚠ %s", term.Sanitize(p.RelayWarning))
			}
			b.WriteString("\n")
		} else {
			fmt.Fprintf(&b, "      live (relay):    %d registered · %d active · %s  [relay]\n",
				p.Registered, p.Active, taskTotalLabel(p.Tasks))
		}
		if p.VaultNote != "" {
			fmt.Fprintf(&b, "      vault (config):  ?  (%s)\n", p.VaultNote)
		} else {
			fmt.Fprintf(&b, "      vault (config):  %s in %d doc injection(s)  [config]\n",
				byteLabel(p.VaultBytes), p.VaultDocs)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func taskTotalLabel(tasks int) string {
	if tasks < 0 {
		return "tasks: ?"
	}
	return fmt.Sprintf("%d active task(s)", tasks)
}

func byteLabel(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}
