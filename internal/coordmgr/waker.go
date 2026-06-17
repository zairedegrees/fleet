package coordmgr

import (
	"fmt"
	"os"
	"strings"
	"sync"
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
	srv  *coord.Server
	wake WakeFunc
	// lastWoken keys are "project/agent". Entries are never evicted: the set is
	// bounded by the number of distinct agents ever woken in this process's life
	// (tens at fleet scale), so unbounded growth is a non-issue, not an oversight.
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
	if err != nil {
		// A real DB read error — distinct from "no channel" (ok == false), which
		// is the normal opt-out path and stays silent.
		fmt.Fprintf(os.Stderr, "waker: notify-channel lookup for %s failed: %v\n", key, err)
		return
	}
	if !ok {
		return
	}
	// notifyChannelTarget filters to `target LIKE 'tmux:%'` in SQL, so the prefix
	// is a schema contract, not defensive trimming — TrimPrefix always strips it.
	session := strings.TrimPrefix(target, "tmux:")
	woke, err := w.wake(req.Project, req.Agent, session)
	if err != nil {
		// Leave lastWoken unset so the 5s sweep retries; surface the failure so a
		// persistently bad session (e.g. wrong registry entry) isn't invisible.
		fmt.Fprintf(os.Stderr, "waker: wake %s in %q failed: %v\n", key, session, err)
		return
	}
	if woke {
		w.lastWoken[key] = w.now()
	}
}

// run drives the waker until stop is closed: instant on each dispatch event,
// plus a reconciliation sweep that recovers dropped events / races / restarts.
// It calls wg.Done on return so Serve can wait for any in-flight sweep query to
// finish before it closes the store (otherwise store.Close could race a read).
func (w *waker) run(stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
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
