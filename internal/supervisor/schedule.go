package supervisor

import (
	"time"

	"github.com/zairedegrees/fleet/internal/config"
)

const (
	backoffThreshold = 2
	maxIntervalSec   = 3600
)

// AgentState is the per-agent supervisor bookkeeping persisted across ticks.
type AgentState struct {
	WakesToday  int       `json:"wakes_today"`
	SpentUSD    float64   `json:"spent_usd"`
	EmptyStreak int       `json:"empty_streak"`
	IntervalSec int       `json:"interval_sec"`
	NextWakeAt  time.Time `json:"next_wake_at"`
}

// State is the per-project supervisor state.
type State struct {
	Date    string                 `json:"date"`
	PID     int                    `json:"pid"`
	Project string                 `json:"project"`
	Agents  map[string]*AgentState `json:"agents"`
}

// BoundedAgent pairs an agent name with its resolved bounded policy.
type BoundedAgent struct {
	Name   string
	Policy config.BoundedPolicy
}

func dayStamp(t time.Time) string { return t.Format("2006-01-02") }

func intervalSeconds(p config.BoundedPolicy) int {
	d, err := time.ParseDuration(p.Interval)
	if err != nil || d <= 0 {
		d = 10 * time.Minute
	}
	return int(d.Seconds())
}

func withinActiveHours(p config.BoundedPolicy, now time.Time) bool {
	if p.ActiveHours == "" {
		return true
	}
	start, end, err := config.ParseActiveHours(p.ActiveHours)
	if err != nil {
		return true // an invalid window fails open; Validate rejects it at load
	}
	mins := now.Hour()*60 + now.Minute()
	return mins >= start && mins < end
}

func exhausted(as *AgentState, p config.BoundedPolicy) bool {
	if p.MaxWakesPerDay > 0 && as.WakesToday >= p.MaxWakesPerDay {
		return true
	}
	if p.BudgetUSD > 0 && as.SpentUSD >= p.BudgetUSD {
		return true
	}
	return false
}

func resetIfNewDay(st *State, now time.Time) {
	d := dayStamp(now)
	if st.Date == d {
		return
	}
	st.Date = d
	for _, as := range st.Agents {
		as.WakesToday = 0
		as.SpentUSD = 0
		as.EmptyStreak = 0
		as.NextWakeAt = now
	}
}

// decideWakes advances state one tick and returns the agents to wake now. It
// charges the wake (count + estimated cost) and sets a provisional next_wake_at
// so a crash before RecordOutcome cannot hot-loop. Pure: now is injected.
func decideWakes(st *State, agents []BoundedAgent, now time.Time) []string {
	if st.Agents == nil {
		st.Agents = map[string]*AgentState{}
	}
	resetIfNewDay(st, now)
	var toWake []string
	for _, ba := range agents {
		as := st.Agents[ba.Name]
		if as == nil {
			as = &AgentState{IntervalSec: intervalSeconds(ba.Policy), NextWakeAt: now}
			st.Agents[ba.Name] = as
		}
		if as.IntervalSec <= 0 {
			as.IntervalSec = intervalSeconds(ba.Policy)
		}
		if !withinActiveHours(ba.Policy, now) || exhausted(as, ba.Policy) || now.Before(as.NextWakeAt) {
			continue
		}
		toWake = append(toWake, ba.Name)
		as.WakesToday++
		as.SpentUSD += ba.Policy.CostPerWakeUSD
		as.NextWakeAt = now.Add(time.Duration(as.IntervalSec) * time.Second)
	}
	return toWake
}

// RecordOutcome applies the productivity feedback after a wake: a productive
// wake resets the cadence to base; an empty one backs off (doubling past the
// threshold, capped). Authoritative over next_wake_at.
func RecordOutcome(st *State, name string, p config.BoundedPolicy, productive bool, now time.Time) {
	as := st.Agents[name]
	if as == nil {
		return
	}
	if productive {
		as.EmptyStreak = 0
		as.IntervalSec = intervalSeconds(p)
	} else {
		as.EmptyStreak++
		if as.EmptyStreak >= backoffThreshold {
			as.IntervalSec *= 2
			if as.IntervalSec > maxIntervalSec {
				as.IntervalSec = maxIntervalSec
			}
		}
	}
	as.NextWakeAt = now.Add(time.Duration(as.IntervalSec) * time.Second)
}
