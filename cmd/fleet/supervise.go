package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/supervisor"
)

const supervisorTick = 30 * time.Second

func newSuperviseCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:    "supervise",
		Short:  "Run the bounded-posture supervisor in the foreground (usually auto-managed)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				cfg, err := loadLastConfig()
				if err != nil {
					return fmt.Errorf("no project for supervise: %w", err)
				}
				project = cfg.Project.Name
			}
			url := flagRelayURL
			if url == "" {
				url = defaultRelayURL
			}
			lf, err := acquireSupervisorLock(project)
			if err != nil {
				fmt.Printf("  %v — not starting a second one\n", err)
				return nil
			}
			defer lf.Close()
			// Record our PID so --kill can find and stop us.
			if err := recordSupervisorPID(project, os.Getpid()); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: could not record supervisor pid: %v\n", err)
			}
			fmt.Printf("  supervisor running for %s (pid %d)\n", project, os.Getpid())
			return supervisor.Run(project, url, supervisorTick)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project to supervise (defaults to last)")
	return cmd
}

func supervisorLockPath(project string) string {
	return filepath.Join(config.FleetDir(), project+".supervisor.lock")
}

// acquireSupervisorLock takes a process-lifetime exclusive flock for the
// project's supervisor (mirrors coordmgr's single-start guard). A non-nil error
// means another supervisor already holds it. Keep the returned file open for as
// long as the supervisor runs; closing it (or process exit) releases the lock.
func acquireSupervisorLock(project string) (*os.File, error) {
	if err := os.MkdirAll(config.FleetDir(), 0o755); err != nil {
		return nil, err
	}
	lf, err := os.OpenFile(supervisorLockPath(project), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lf.Close()
		return nil, fmt.Errorf("supervisor already running for %s", project)
	}
	return lf, nil
}

// recordSupervisorPID stamps our PID into the project's supervisor state so
// `fleet --kill` can find us. A corrupt/unreadable state file (LoadState
// returns nil,err) must not panic — fall back to a fresh state — and a failed
// write is surfaced, not swallowed, since a missing PID makes us unstoppable.
func recordSupervisorPID(project string, pid int) error {
	st, err := supervisor.LoadState(project)
	if err != nil || st == nil {
		st = &supervisor.State{Project: project, Agents: map[string]*supervisor.AgentState{}}
	}
	st.PID = pid
	return supervisor.SaveState(st)
}

// spawnSupervisor re-execs `fleet supervise` detached so it survives fleet's
// exit. No-op when no agent is bounded or a live supervisor already runs.
func spawnSupervisor(cfg *config.FleetConfig) error {
	if boundedCount(cfg) == 0 {
		return nil
	}
	if alive(supervisorPID(cfg.Project.Name)) {
		return nil // single instance
	}
	logPath := filepath.Join(config.FleetDir(), cfg.Project.Name+".supervisor.log")
	logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	args := []string{"supervise", "--project", cfg.Project.Name}
	if flagRelayURL != "" {
		args = append(args, "--relay-url", flagRelayURL)
	}
	c := exec.Command(os.Args[0], args...)
	if logFile != nil {
		c.Stdout, c.Stderr = logFile, logFile
	}
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c.Start()
}

// stopSupervisor signals the project's supervisor process group and clears its
// state. Safe to call when none is running.
func stopSupervisor(project string) {
	pid := supervisorPID(project)
	if alive(pid) {
		_ = syscall.Kill(-pid, syscall.SIGTERM) // negative pid → the process group
	}
	_ = supervisor.ClearState(project)
}

func boundedCount(cfg *config.FleetConfig) int {
	n := 0
	for _, a := range cfg.Agents {
		if a.IsBounded() {
			n++
		}
	}
	return n
}

func supervisorPID(project string) int {
	st, err := supervisor.LoadState(project)
	if err != nil {
		return 0
	}
	return st.PID
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
