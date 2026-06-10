package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

type LaunchResult struct {
	Agent   string
	Action  string // "created", "skipped", "restarted", "failed"
	Success bool
	Error   error
}

// CreateSessions creates tmux sessions and launches Claude Code in each.
// Returns immediately — does NOT wait for Claude to boot. claudeBin is the
// resolved absolute path to the Claude Code binary (see config.ResolveBin).
func CreateSessions(cfg *config.FleetConfig, claudeBin string) []LaunchResult {
	var results []LaunchResult
	project := cfg.Project.Name

	claudeCmd := claudeBin
	for _, f := range cfg.Claude.Flags {
		claudeCmd += " " + f
	}

	for _, agent := range cfg.Agents {
		res := LaunchResult{Agent: agent.Name}

		if HasSession(project, agent.Name) {
			fmt.Printf("  ✓ %s already running, skipping\n", SessionName(project, agent.Name))
			res.Success = true
			res.Action = "skipped"
			results = append(results, res)
			continue
		}

		if err := CreateSession(project, agent.Name, cfg.Project.Cwd); err != nil {
			res.Error = fmt.Errorf("tmux create failed: %w", err)
			res.Action = "failed"
			results = append(results, res)
			continue
		}

		// Explicit cd to ensure Claude starts in the right directory
		if err := SendKeys(project, agent.Name, "cd "+cfg.Project.Cwd); err != nil {
			res.Error = fmt.Errorf("cd failed: %w", err)
			res.Action = "failed"
			results = append(results, res)
			continue
		}
		time.Sleep(200 * time.Millisecond)

		if err := SendKeys(project, agent.Name, claudeCmd); err != nil {
			res.Error = fmt.Errorf("claude launch failed: %w", err)
			res.Action = "failed"
			results = append(results, res)
			continue
		}

		res.Success = true
		res.Action = "created"
		results = append(results, res)
	}

	return results
}

// ConfigureAgentsAsync registers the fleet on the relay synchronously (the
// HTTP calls don't depend on pane readiness), then generates a shell script
// for the pane-dependent configuration (prompt wait + send-keys) and launches
// it as a detached process that survives fleet exit.
// Logs output to ~/.fleet/logs/configure-{timestamp}.log
func ConfigureAgentsAsync(cfg *config.FleetConfig) (string, error) {
	// wake.sh is independent of the configure run; generate it up front.
	generateWakeScript(cfg)
	relayURL := cfg.Project.RelayURL
	if relayURL == "" {
		relayURL = config.DefaultRelayURL
	}
	// Registration runs synchronously before fleet exits: keep the timeout
	// short so a hanging relay can't block the launch for 10s per call.
	return configureAgents(cfg, config.FleetDir(), spawnDetached, relay.NewClientWithTimeout(relayURL, registerTimeout))
}

// spawnDetached starts the configure script as a detached process that survives
// fleet exit.
func spawnDetached(scriptPath string) error {
	cmd := execCommand("bash", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

// configureAgents registers the fleet on the relay, then writes the configure
// script under fleetDir and runs it via spawn, returning the log path plus any
// registration/setup/spawn error (joined — a registration failure must surface
// but never blocks the pane-dependent script). Taking fleetDir, spawn and rc as
// parameters makes the error paths unit-testable without touching ~/.fleet,
// running bash, or needing a relay.
func configureAgents(cfg *config.FleetConfig, fleetDir string, spawn func(string) error, rc relayRegistrar) (string, error) {
	relayErr := registerFleet(cfg, rc)

	logDir := filepath.Join(fleetDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", errors.Join(relayErr, fmt.Errorf("create log dir: %w", err))
	}
	rotateConfigLogs(logDir, 5)

	timestamp := time.Now().Format("20060102-150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("configure-%s.log", timestamp))
	scriptPath := filepath.Join(fleetDir, "configure-agents.sh")

	if err := os.WriteFile(scriptPath, []byte(buildConfigureScript(cfg, logPath)), 0755); err != nil {
		return logPath, errors.Join(relayErr, fmt.Errorf("write configure script: %w", err))
	}

	if err := spawn(scriptPath); err != nil {
		return logPath, errors.Join(relayErr, fmt.Errorf("start configure script: %w", err))
	}
	return logPath, relayErr
}

// buildConfigureScript returns the bash script for the pane-dependent agent
// configuration after Claude boots: rename/color, the in-pane /relay register
// skill command, the register_agent re-assert curl, and — only for agents with
// AutoTalk=true — the continuous `/relay talk` poll loop. Profile + vault HTTP
// lives in registerFleet via the typed client; the script must stay detached
// because it waits up to ~90s per pane and must survive fleet's exit.
// Kept pure so the talk-gating logic is unit-testable.
func buildConfigureScript(cfg *config.FleetConfig, logPath string) string {
	relayURL := cfg.Project.RelayURL
	if relayURL == "" {
		relayURL = config.DefaultRelayURL
	}

	var script strings.Builder
	script.WriteString("#!/bin/bash\n")
	script.WriteString(fmt.Sprintf("exec > %s 2>&1\n", logPath))
	script.WriteString(fmt.Sprintf("RELAY_URL=\"%s\"\n", relayURL))
	script.WriteString("wait_prompt() {\n")
	script.WriteString("  local session=$1 timeout=$2 elapsed=0\n")
	script.WriteString("  while [ $elapsed -lt $timeout ]; do\n")
	script.WriteString("    if tmux capture-pane -t \"$session\" -p 2>/dev/null | grep -q '❯'; then\n")
	script.WriteString("      return 0\n")
	script.WriteString("    fi\n")
	script.WriteString("    sleep 1\n")
	script.WriteString("    elapsed=$((elapsed + 1))\n")
	script.WriteString("  done\n")
	script.WriteString("  return 1\n")
	script.WriteString("}\n\n")

	// Configure each agent
	project := cfg.Project.Name
	for _, agent := range cfg.Agents {
		session := SessionName(project, agent.Name)
		escapedRole := strings.ReplaceAll(agent.Role, "'", "'\\''")

		script.WriteString(fmt.Sprintf("# Configure %s\n", agent.Name))
		script.WriteString(fmt.Sprintf("if wait_prompt %s 90; then\n", session))
		script.WriteString(fmt.Sprintf("  tmux send-keys -t %s '/rename %s' Enter\n", session, agent.Name))
		script.WriteString("  sleep 2\n")
		script.WriteString(fmt.Sprintf("  tmux send-keys -t %s '/color %s' Enter\n", session, agent.Color))
		script.WriteString("  sleep 2\n")

		registerCmd := fmt.Sprintf("/relay register %s %s %s", agent.Name, cfg.Project.Name, escapedRole)
		if agent.ReportsTo != "" {
			registerCmd += " Reports to " + agent.ReportsTo + "."
		}
		// Type the command, let the input + skill autocomplete settle, then submit
		// with a SEPARATE Enter. A long /relay command sent as '...' Enter in one
		// keystroke is typed but never submitted (the Enter is swallowed).
		script.WriteString(fmt.Sprintf("  tmux send-keys -t %s '%s'\n", session, strings.ReplaceAll(registerCmd, "'", "'\\''")))
		script.WriteString("  sleep 1\n")
		script.WriteString(fmt.Sprintf("  tmux send-keys -t %s Enter\n", session))
		script.WriteString("  sleep 3\n")

		// The in-pane /relay register above makes the agent's LLM call
		// register_agent WITHOUT profile_slug/reports_to/is_executive, and the
		// relay's re-register is a full-replace UPDATE that NULLs them — making
		// dispatched tasks invisible to the agent. Re-assert the complete
		// registration here so this curl is the LAST write (the battle-tested
		// order from main). Do not remove it for protocol purity.
		script.WriteString(fmt.Sprintf("  curl -s -X POST \"$RELAY_URL\" -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"register_agent\",\"arguments\":{\"name\":\"%s\",\"project\":\"%s\",\"role\":\"%s\",\"profile_slug\":\"%s\",\"reports_to\":\"%s\",\"is_executive\":%t}}}' > /dev/null 2>&1\n",
			agent.Name, cfg.Project.Name, strings.ReplaceAll(agent.Role, "\"", "\\\""), agent.Name, agent.ReportsTo, agent.IsExecutive))
		script.WriteString("  sleep 1\n")

		if agent.AutoTalk {
			script.WriteString(fmt.Sprintf("  wait_prompt %s 15\n", session))
			script.WriteString(fmt.Sprintf("  tmux send-keys -t %s '/relay talk'\n", session))
			script.WriteString("  sleep 1\n")
			script.WriteString(fmt.Sprintf("  tmux send-keys -t %s Enter\n", session))
		}

		script.WriteString(fmt.Sprintf("  echo '✓ %s configured'\n", session))
		script.WriteString("else\n")
		script.WriteString(fmt.Sprintf("  echo '⚠ %s: timeout'\n", session))
		script.WriteString("fi\n\n")
	}

	script.WriteString("echo 'all agents configured'\n")

	return script.String()
}

// generateWakeScript creates ~/.fleet/wake.sh for boss agents to wake workers.
// Usage from Claude Code: ! bash ~/.fleet/wake.sh dev
// Usage to wake all:      ! bash ~/.fleet/wake.sh --all
func generateWakeScript(cfg *config.FleetConfig) {
	project := cfg.Project.Name
	wakePath := filepath.Join(config.FleetDir(), "wake.sh")

	var w strings.Builder
	w.WriteString("#!/bin/bash\n")
	w.WriteString("# Auto-generated by fleet — wake agents via tmux\n")
	w.WriteString(fmt.Sprintf("# Project: %s\n\n", project))

	w.WriteString("if [ \"$1\" = \"--all\" ]; then\n")
	for _, agent := range cfg.Agents {
		if !agent.IsExecutive {
			session := SessionName(project, agent.Name)
			w.WriteString(fmt.Sprintf("  tmux send-keys -t %s '/relay talk' Enter 2>/dev/null && echo '  ✓ %s woken' || echo '  ⚠ %s: no session'\n",
				session, agent.Name, agent.Name))
		}
	}
	w.WriteString("  exit 0\n")
	w.WriteString("fi\n\n")

	w.WriteString("if [ -z \"$1\" ]; then\n")
	w.WriteString("  echo 'Usage: bash ~/.fleet/wake.sh <agent-name>'\n")
	w.WriteString("  echo '       bash ~/.fleet/wake.sh --all'\n")
	w.WriteString("  echo ''\n")
	w.WriteString("  echo 'Available agents:'\n")
	for _, agent := range cfg.Agents {
		if !agent.IsExecutive {
			w.WriteString(fmt.Sprintf("  echo '  %s'\n", agent.Name))
		}
	}
	w.WriteString("  exit 1\n")
	w.WriteString("fi\n\n")

	w.WriteString(fmt.Sprintf("SESSION=\"fleet-%s-$1\"\n", project))
	w.WriteString("tmux send-keys -t \"$SESSION\" '/relay talk' Enter 2>/dev/null && echo \"  ✓ $1 woken\" || echo \"  ⚠ $1: no session\"\n")

	os.WriteFile(wakePath, []byte(w.String()), 0755)
}

// rotateConfigLogs keeps only the N most recent log files.
func rotateConfigLogs(logDir string, keep int) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	var logs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "configure-") && strings.HasSuffix(e.Name(), ".log") {
			logs = append(logs, e.Name())
		}
	}
	// Files are naturally sorted by timestamp in name
	if len(logs) > keep {
		for _, name := range logs[:len(logs)-keep] {
			os.Remove(filepath.Join(logDir, name))
		}
	}
}

// WakeAgent is defined in tmux.go
