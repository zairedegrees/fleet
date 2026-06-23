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
			// Record our PID so --kill can find and stop us.
			st, _ := supervisor.LoadState(project)
			st.PID = os.Getpid()
			_ = supervisor.SaveState(st)
			fmt.Printf("  supervisor running for %s (pid %d)\n", project, os.Getpid())
			return supervisor.Run(project, url, supervisorTick)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project to supervise (defaults to last)")
	return cmd
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
