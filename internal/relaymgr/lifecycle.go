package relaymgr

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// seams
var (
	startServer = defaultStartServer
	ensureBin   = EnsureBinary
	killProc    = syscall.Kill
	procCommand = defaultProcCommand
)

// defaultProcCommand returns the full command line of a running pid (empty when
// the pid is gone). Cross-platform via ps, used to confirm a pidfile still points
// at agent-relay before signalling it.
func defaultProcCommand(pid int) string {
	out, err := runCmd("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// EnsureRunning guarantees a relay answers at url. If one is already reachable it
// returns immediately (a real wrai.th or an already-managed instance). Otherwise,
// for the default local url (isExternal=false) it acquires the binary and starts
// a managed instance; for an external --relay-url it errors instead of managing
// someone else's relay. Caller obtains download consent before first use.
func EnsureRunning(url string, isExternal bool) error {
	if Reachable(url) {
		return nil
	}
	if isExternal {
		return fmt.Errorf("relay unreachable at %s (external --relay-url is not auto-managed)", url)
	}
	bin, err := ensureBin()
	if err != nil {
		return err
	}
	if err := startServer(bin, url); err != nil {
		return err
	}
	for i := 0; i < 30; i++ {
		if Reachable(url) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("managed relay did not become reachable at %s", url)
}

// defaultStartServer spawns `agent-relay serve` detached, guarded by a flock so
// concurrent fleet commands don't double-start, and records a pidfile.
func defaultStartServer(bin, url string) error {
	lf, err := os.OpenFile(lockPath(), os.O_CREATE|os.O_RDWR, 0644)
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
	cmd := runCmd(bin, "serve")
	cmd.Env = append(os.Environ(), "PORT="+portFromURL(url))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start agent-relay: %w", err)
	}
	return os.WriteFile(pidPath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
}

// Stop signals the managed relay (no-op if none).
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
	// Only signal the pid if it still points at agent-relay — a stale pidfile
	// whose pid was recycled by the OS must not get an unrelated process killed.
	if strings.Contains(procCommand(pid), "agent-relay") {
		_ = killProc(pid, syscall.SIGTERM)
	}
	return os.Remove(pidPath())
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
