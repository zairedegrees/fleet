package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BoundedPolicy bounds a "bounded"-posture agent's proactive re-wakes. A zero
// field means "inherit" during resolution (builtin defaults <- [bounded_defaults]
// <- per-agent).
type BoundedPolicy struct {
	Interval       string  `toml:"interval,omitempty"`
	ActiveHours    string  `toml:"active_hours,omitempty"`
	MaxWakesPerDay int     `toml:"max_wakes_per_day,omitempty"`
	BudgetUSD      float64 `toml:"budget_usd,omitempty"`
	CostPerWakeUSD float64 `toml:"cost_per_wake_usd,omitempty"`
}

// DefaultBoundedPolicy is the built-in base every bounded agent inherits.
// ActiveHours "" means 24h. CostPerWakeUSD is a labelled estimate, not measured.
var DefaultBoundedPolicy = BoundedPolicy{
	Interval:       "10m",
	ActiveHours:    "",
	MaxWakesPerDay: 50,
	BudgetUSD:      3.00,
	CostPerWakeUSD: 0.06,
}

// ResolveBounded layers builtin defaults <- [bounded_defaults] <- per-agent.
func (cfg *FleetConfig) ResolveBounded(a AgentConfig) BoundedPolicy {
	p := DefaultBoundedPolicy
	if cfg.BoundedDefaults != nil {
		p = mergeBounded(p, *cfg.BoundedDefaults)
	}
	if a.Bounded != nil {
		p = mergeBounded(p, *a.Bounded)
	}
	return p
}

// mergeBounded overlays non-zero fields of over onto base.
func mergeBounded(base, over BoundedPolicy) BoundedPolicy {
	if over.Interval != "" {
		base.Interval = over.Interval
	}
	if over.ActiveHours != "" {
		base.ActiveHours = over.ActiveHours
	}
	if over.MaxWakesPerDay != 0 {
		base.MaxWakesPerDay = over.MaxWakesPerDay
	}
	if over.BudgetUSD != 0 {
		base.BudgetUSD = over.BudgetUSD
	}
	if over.CostPerWakeUSD != 0 {
		base.CostPerWakeUSD = over.CostPerWakeUSD
	}
	return base
}

func (p BoundedPolicy) Validate() error {
	if p.Interval != "" {
		if _, err := time.ParseDuration(p.Interval); err != nil {
			return fmt.Errorf("invalid interval %q: %w", p.Interval, err)
		}
	}
	if p.ActiveHours != "" {
		if _, _, err := ParseActiveHours(p.ActiveHours); err != nil {
			return err
		}
	}
	if p.MaxWakesPerDay < 0 {
		return fmt.Errorf("max_wakes_per_day must be >= 0")
	}
	if p.BudgetUSD < 0 {
		return fmt.Errorf("budget_usd must be >= 0")
	}
	if p.CostPerWakeUSD < 0 {
		return fmt.Errorf("cost_per_wake_usd must be >= 0")
	}
	return nil
}

// ParseActiveHours parses "HH:MM-HH:MM" into minutes-after-midnight. v1 requires
// start < end (no cross-midnight window).
func ParseActiveHours(s string) (startMin, endMin int, err error) {
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("invalid active_hours %q: want HH:MM-HH:MM", s)
	}
	startMin, err = parseHHMM(a)
	if err != nil {
		return 0, 0, err
	}
	endMin, err = parseHHMM(b)
	if err != nil {
		return 0, 0, err
	}
	if startMin >= endMin {
		return 0, 0, fmt.Errorf("invalid active_hours %q: start must be before end (no cross-midnight)", s)
	}
	return startMin, endMin, nil
}

func parseHHMM(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return 0, fmt.Errorf("invalid time %q: want HH:MM", s)
	}
	hi, err := strconv.Atoi(h)
	if err != nil || hi < 0 || hi > 23 {
		return 0, fmt.Errorf("invalid hour in %q", s)
	}
	mi, err := strconv.Atoi(strings.TrimSpace(m))
	if err != nil || mi < 0 || mi > 59 {
		return 0, fmt.Errorf("invalid minute in %q", s)
	}
	return hi*60 + mi, nil
}
