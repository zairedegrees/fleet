package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
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

// ConfigureAgentsAsync generates a shell script that configures agents after
// Claude boots, then launches it as a detached process that survives fleet exit.
// Logs output to ~/.fleet/logs/configure-{timestamp}.log
func ConfigureAgentsAsync(cfg *config.FleetConfig) (string, error) {
	// wake.sh is independent of the configure run; generate it up front.
	generateWakeScript(cfg)
	return configureAgents(cfg, config.FleetDir(), spawnDetached)
}

// spawnDetached starts the configure script as a detached process that survives
// fleet exit.
func spawnDetached(scriptPath string) error {
	cmd := exec.Command("bash", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

// configureAgents writes the configure script under fleetDir and runs it via
// spawn, returning the log path plus any setup/spawn error. Taking fleetDir and
// spawn as parameters makes the error paths unit-testable without touching
// ~/.fleet or actually running bash.
func configureAgents(cfg *config.FleetConfig, fleetDir string, spawn func(string) error) (string, error) {
	logDir := filepath.Join(fleetDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("create log dir: %w", err)
	}
	rotateConfigLogs(logDir, 5)

	timestamp := time.Now().Format("20060102-150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("configure-%s.log", timestamp))
	scriptPath := filepath.Join(fleetDir, "configure-agents.sh")

	relayURL := cfg.Project.RelayURL
	if relayURL == "" {
		relayURL = "http://localhost:8090/mcp"
	}

	if err := os.WriteFile(scriptPath, []byte(buildConfigureScript(cfg, relayURL, logPath)), 0755); err != nil {
		return logPath, fmt.Errorf("write configure script: %w", err)
	}

	if err := spawn(scriptPath); err != nil {
		return logPath, fmt.Errorf("start configure script: %w", err)
	}
	return logPath, nil
}

// buildConfigureScript returns the bash script that configures each agent after
// Claude boots: rename/color, relay register + profile_slug, vault injection, and
// — only for agents with AutoTalk=true — the continuous `/relay talk` poll loop.
// Kept pure (reads only vault docs) so the talk-gating logic is unit-testable.
func buildConfigureScript(cfg *config.FleetConfig, relayURL, logPath string) string {
	var script strings.Builder
	script.WriteString("#!/bin/bash\n")
	script.WriteString(fmt.Sprintf("exec > %s 2>&1\n", logPath))
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

	// Ensure profiles via relay
	script.WriteString(fmt.Sprintf("RELAY_URL=\"%s\"\n", relayURL))
	script.WriteString(`ensure_profile() {
  local slug="$1" name="$2" role="$3" project="$4"
  curl -s -X POST "$RELAY_URL" \
    -H "Content-Type: application/json" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"register_profile\",\"arguments\":{\"slug\":\"$slug\",\"name\":\"$name\",\"role\":\"$role\",\"project\":\"$project\"}}}" \
    > /dev/null 2>&1
}
`)

	for _, agent := range cfg.Agents {
		escapedName := strings.ReplaceAll(agent.Name, "'", "'\\''")
		escapedRole := strings.ReplaceAll(agent.Role, "\"", "\\\"")
		script.WriteString(fmt.Sprintf("ensure_profile '%s' '%s' '%s' '%s'\n",
			escapedName, escapedName, escapedRole, cfg.Project.Name))
	}
	script.WriteString("\n")

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

		// Set profile_slug via direct relay call — without this, agents can't see dispatched tasks
		script.WriteString(fmt.Sprintf("  curl -s -X POST \"$RELAY_URL\" -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"register_agent\",\"arguments\":{\"name\":\"%s\",\"project\":\"%s\",\"role\":\"%s\",\"profile_slug\":\"%s\"}}}' > /dev/null 2>&1\n",
			agent.Name, cfg.Project.Name, strings.ReplaceAll(agent.Role, "\"", "\\\""), agent.Name))
		script.WriteString("  sleep 1\n")

		// Vault injection via relay
		vaultDir := filepath.Join(cfg.Project.Cwd, ".fleet", "vault")
		docs, err := config.ResolveVaultDocs(vaultDir, agent)
		if err == nil && len(docs) > 0 {
			totalSize := config.VaultSize(docs)
			if totalSize > int64(config.VaultSizeWarningBytes) {
				script.WriteString(fmt.Sprintf("  echo 'WARNING: vault for %s is %dKB (>50KB)'\n", agent.Name, totalSize/1024))
			}
			for _, doc := range docs {
				escapedContent := strings.ReplaceAll(string(doc.Content), "'", "'\\''")
				escapedContent = strings.ReplaceAll(escapedContent, "\n", "\\n")
				script.WriteString(fmt.Sprintf("  curl -s -X POST \"$RELAY_URL\" -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"set_memory\",\"arguments\":{\"key\":\"vault:%s\",\"value\":\"%s\",\"scope\":\"project\",\"project\":\"%s\",\"tags\":[\"vault\",\"auto-injected\"]}}}' > /dev/null 2>&1\n",
					doc.Path, strings.ReplaceAll(escapedContent, "\"", "\\\""), cfg.Project.Name))
			}
			script.WriteString(fmt.Sprintf("  echo 'vault injected for %s: %d docs'\n", agent.Name, len(docs)))
		}

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
