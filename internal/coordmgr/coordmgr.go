// Package coordmgr lifecycle-manages the embedded coordination core: it runs the
// coord server as a detached `fleet coord serve` child that outlives the launch
// command (agents keep talking to it after fleet exits), with a pidfile + flock
// so concurrent fleet commands don't double-start it. Unlike relaymgr it
// downloads nothing — coord is compiled into the fleet binary (MIT, no AGPL).
package coordmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/coord"
	"github.com/zairedegrees/fleet/internal/relay"
)

// seams (swapped in tests)
var (
	probe       = defaultProbe
	startServer = defaultStartServer
	killProc    = syscall.Kill
	procCommand = defaultProcCommand
	runCmd      = exec.Command
)

func defaultProbe(url string) error { return relay.NewClient(url).Health() }

func pidPath() string  { return filepath.Join(config.FleetDir(), "coord.pid") }
func lockPath() string { return filepath.Join(config.FleetDir(), "coord.lock") }

// DBPath is the coord SQLite database path (separate from any relay state).
func DBPath() string { return filepath.Join(config.FleetDir(), "coord.db") }

// Reachable reports whether coord answers at url (a bounded list_orgs probe, not
// a bare GET — the /mcp endpoint declines GET).
func Reachable(url string) bool { return probe(url) == nil }

// Serve opens the coord store and serves the coordination API on port, blocking
// until the process is signalled. On SIGTERM/SIGINT it drains in-flight requests
// (coord.Shutdown) and closes the store so the WAL is finalized — this is the
// body of `fleet coord serve`, the detached child Stop() signals.
//
// wake is an optional WakeFunc injected by cmd/fleet; pass nil to disable the
// waker (coordmgr must not import runner to avoid an import cycle).
func Serve(port string, wake WakeFunc) error {
	if err := os.MkdirAll(config.FleetDir(), 0o755); err != nil {
		return err
	}
	store, err := coord.OpenStore(DBPath())
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	srv := coord.New(store)

	// Stop the waker and wait for any in-flight sweep query to finish BEFORE the
	// store is closed. This defer is registered after store.Close so it runs
	// first (LIFO), closing the read/store-access race window on shutdown.
	stop := make(chan struct{})
	var wakerWG sync.WaitGroup
	defer func() {
		close(stop)
		wakerWG.Wait()
	}()
	if wake != nil {
		w := &waker{srv: srv, wake: wake, lastWoken: map[string]time.Time{}, now: time.Now}
		wakerWG.Add(1)
		go w.run(stop, &wakerWG)
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(":" + port) }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	select {
	case err := <-serveErr:
		return err
	case <-sig:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

// EnsureRunning guarantees coord answers at url, starting a detached child if
// needed. An external --relay-url is never auto-managed.
func EnsureRunning(url string, isExternal bool) error {
	if Reachable(url) {
		return nil
	}
	if isExternal {
		return fmt.Errorf("relay unreachable at %s (external --relay-url is not auto-managed)", url)
	}
	if err := startServer(url); err != nil {
		return err
	}
	// First run creates the DB + runs the full DDL before binding, so allow a
	// slightly longer budget than the prebuilt-binary path.
	for i := 0; i < 50; i++ {
		if Reachable(url) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("embedded coord did not become reachable at %s", url)
}

// defaultStartServer spawns `fleet coord serve` detached, flock-guarded, and
// records the child pid.
func defaultStartServer(url string) error {
	lf, err := os.OpenFile(lockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		// another fleet process is already starting it; let the caller's poll see it
		return nil
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)

	if Reachable(url) {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	cmd := runCmd(self, "coord", "serve")
	cmd.Env = append(os.Environ(), "PORT="+portFromURL(url))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start coord: %w", err)
	}
	return os.WriteFile(pidPath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
}

// Stop signals the detached coord child (no-op if none). The ps-guard checks the
// pid still runs `coord serve` so a recycled pid is never signalled.
func Stop() error {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return err
	}
	if strings.Contains(procCommand(pid), "coord serve") {
		_ = killProc(pid, syscall.SIGTERM)
	}
	return os.Remove(pidPath())
}

func defaultProcCommand(pid int) string {
	out, err := runCmd("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// portFromURL extracts the port from http://host:PORT/path, defaulting to 8090.
func portFromURL(url string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		return s[i+1:]
	}
	return "8090"
}
