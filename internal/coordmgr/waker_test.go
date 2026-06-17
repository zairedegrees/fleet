package coordmgr

import (
	"sync"
	"testing"
	"time"

	"github.com/zairedegrees/fleet/internal/coord"
)

func newWakerTestServer(t *testing.T) *coord.Server {
	t.Helper()
	store, err := coord.OpenStore(t.TempDir() + "/coord.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return coord.New(store)
}

func seedChannel(t *testing.T, srv *coord.Server, project, agent, target string) {
	t.Helper()
	if err := srv.RegisterNotifyChannelForTest(project, agent, target); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
}

func TestWakerWakesWhenChannelRegistered(t *testing.T) {
	srv := newWakerTestServer(t)
	seedChannel(t, srv, "acme", "worker", "tmux:fleet-acme-worker")

	var woke []string
	w := &waker{
		srv:       srv,
		lastWoken: map[string]time.Time{},
		now:       func() time.Time { return time.Unix(1000, 0) },
		wake: func(project, agent, session string) (bool, error) {
			woke = append(woke, project+"/"+agent+"/"+session)
			return true, nil
		},
	}
	w.tryWake(coord.WakeRequest{Project: "acme", Agent: "worker"})

	if len(woke) != 1 || woke[0] != "acme/worker/fleet-acme-worker" {
		t.Fatalf("want one wake with stripped session, got %v", woke)
	}
}

func TestWakerSkipsWithoutChannel(t *testing.T) {
	srv := newWakerTestServer(t)
	var woke int
	w := &waker{srv: srv, lastWoken: map[string]time.Time{}, now: time.Now,
		wake: func(_, _, _ string) (bool, error) { woke++; return true, nil }}
	w.tryWake(coord.WakeRequest{Project: "acme", Agent: "noChannel"})
	if woke != 0 {
		t.Fatalf("no channel must mean no wake, got %d", woke)
	}
}

func TestWakerCooldown(t *testing.T) {
	srv := newWakerTestServer(t)
	seedChannel(t, srv, "acme", "worker", "tmux:fleet-acme-worker")
	now := time.Unix(1000, 0)
	var woke int
	w := &waker{srv: srv, lastWoken: map[string]time.Time{},
		now:  func() time.Time { return now },
		wake: func(_, _, _ string) (bool, error) { woke++; return true, nil }}

	w.tryWake(coord.WakeRequest{Project: "acme", Agent: "worker"})
	w.tryWake(coord.WakeRequest{Project: "acme", Agent: "worker"}) // within cooldown
	if woke != 1 {
		t.Fatalf("cooldown must suppress the 2nd wake, got %d", woke)
	}
	now = now.Add(wakeCooldown + time.Second)
	w.tryWake(coord.WakeRequest{Project: "acme", Agent: "worker"})
	if woke != 2 {
		t.Fatalf("past cooldown must wake again, got %d", woke)
	}
}

// TestWakerRunStopsAndSignalsWaitGroup pins the shutdown contract Serve relies
// on: closing stop must end run(), and run() must call wg.Done so Serve can wait
// for an in-flight sweep before closing the store.
func TestWakerRunStopsAndSignalsWaitGroup(t *testing.T) {
	srv := newWakerTestServer(t)
	w := &waker{srv: srv, lastWoken: map[string]time.Time{}, now: time.Now,
		wake: func(_, _, _ string) (bool, error) { return true, nil }}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go w.run(stop, &wg)

	close(stop)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return / WaitGroup not signalled after stop closed")
	}
}
