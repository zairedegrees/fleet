package supervisor

import (
	"fmt"
	"os"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
	"github.com/zairedegrees/fleet/internal/runner"
)

// Deps are the injectable side-effects of one tick, making the loop testable
// without tmux or a relay.
type Deps struct {
	Wake       func(project, agent string) (bool, error)
	Productive func(project, agent string) bool
	Now        func() time.Time
}

// tick runs one scheduling iteration: load state, decide wakes, wake + measure
// each, record the outcome, persist.
func tick(project string, agents []BoundedAgent, deps Deps) error {
	st, err := LoadState(project)
	if err != nil {
		return err
	}
	now := deps.Now()
	for _, name := range decideWakes(st, agents, now) {
		woke, werr := deps.Wake(project, name) // never fatal; a busy/ghost pane is just "not woken"
		if werr != nil {
			fmt.Fprintf(os.Stderr, "  supervisor: wake %s failed: %v\n", name, werr)
		}
		if !woke {
			// Phantom wake: refund the provisional charge decideWakes made and
			// retry on the next tick rather than burning the daily cap.
			if as := st.Agents[name]; as != nil {
				as.WakesToday--
				as.SpentUSD -= policyFor(agents, name).CostPerWakeUSD
				as.NextWakeAt = now
			}
			continue
		}
		productive := deps.Productive(project, name)
		RecordOutcome(st, name, policyFor(agents, name), productive, now)
	}
	return SaveState(st)
}

func policyFor(agents []BoundedAgent, name string) config.BoundedPolicy {
	for _, ba := range agents {
		if ba.Name == name {
			return ba.Policy
		}
	}
	return config.DefaultBoundedPolicy
}

// Run loads the project's bounded agents once and ticks forever. Real Deps wire
// tmux wakes and a relay task-count probe for the productivity signal.
func Run(project, relayURL string, interval time.Duration) error {
	cfg, err := config.LoadLast()
	if err != nil {
		return err
	}
	var agents []BoundedAgent
	for _, a := range cfg.Agents {
		if a.IsBounded() {
			agents = append(agents, BoundedAgent{Name: a.Name, Policy: cfg.ResolveBounded(a)})
		}
	}
	client := relay.NewClientWithTimeout(relayURL, 5*time.Second)
	deps := Deps{
		Wake: func(project, agent string) (bool, error) {
			return runner.WakeSessionIfDormant(runner.SessionName(project, agent), agent, project)
		},
		Productive: func(project, agent string) bool {
			n, err := client.CountActiveTasks(project, agent)
			return err == nil && n > 0
		},
		Now: time.Now,
	}
	for {
		// A transient state/io error must not kill autonomy — keep ticking.
		_ = tick(project, agents, deps)
		time.Sleep(interval)
	}
}
