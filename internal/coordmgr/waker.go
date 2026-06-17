package coordmgr

import (
	"strings"
	"time"

	"github.com/zairedegrees/fleet/internal/coord"
)

const (
	wakeCooldown  = 20 * time.Second
	sweepInterval = 5 * time.Second
)

// WakeFunc attempts to wake a dormant agent in `session`; returns whether it
// woke. Injected by cmd/fleet so coordmgr never imports runner (tmux).
type WakeFunc func(project, agent, session string) (bool, error)

type waker struct {
	srv       *coord.Server
	wake      WakeFunc
	lastWoken map[string]time.Time
	now       func() time.Time
}

// tryWake resolves the agent's tmux channel and wakes it, honoring the cooldown
// and the opt-in (no channel → not fleet-wakeable → skip).
func (w *waker) tryWake(req coord.WakeRequest) {
	key := req.Project + "/" + req.Agent
	if t, ok := w.lastWoken[key]; ok && w.now().Sub(t) < wakeCooldown {
		return
	}
	target, ok, err := w.srv.NotifyChannelTarget(req.Project, req.Agent)
	if err != nil || !ok {
		return
	}
	session := strings.TrimPrefix(target, "tmux:")
	woke, err := w.wake(req.Project, req.Agent, session)
	if err == nil && woke {
		w.lastWoken[key] = w.now()
	}
}

// run drives the waker until stop is closed: instant on each dispatch event,
// plus a reconciliation sweep that recovers dropped events / races / restarts.
func (w *waker) run(stop <-chan struct{}) {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case req := <-w.srv.Dispatched():
			w.tryWake(req)
		case <-ticker.C:
			reqs, err := w.srv.AgentsWithPendingTasks()
			if err != nil {
				continue
			}
			for _, req := range reqs {
				w.tryWake(req)
			}
		}
	}
}
