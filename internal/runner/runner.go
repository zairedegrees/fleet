package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nazaire/fleet/internal/config"
)

type LaunchResult struct {
	Agent   string
	Action  string // "created", "skipped", "restarted", "failed"
	Success bool
	Error   error
}

// CreateSessions creates tmux sessions and launches Claude Code in each.
// Returns immediately — does NOT wait for Claude to boot.
func CreateSessions(cfg *config.FleetConfig) []LaunchResult {
	var results []LaunchResult

	claudeCmd := "claude"
	for _, f := range cfg.Claude.Flags {
		claudeCmd += " " + f
	}

	for _, agent := range cfg.Agents {
		res := LaunchResult{Agent: agent.Name}

		if HasSession(agent.Name) {
			fmt.Printf("  ✓ %s already running, skipping\n", SessionName(agent.Name))
			res.Success = true
			res.Action = "skipped"
			results = append(results, res)
			continue
		}

		if err := CreateSession(agent.Name, cfg.Project.Cwd); err != nil {
			res.Error = fmt.Errorf("tmux create failed: %w", err)
			res.Action = "failed"
			results = append(results, res)
			continue
		}

		// Explicit cd to ensure Claude starts in the right directory
		if err := SendKeys(agent.Name, "cd "+cfg.Project.Cwd); err != nil {
			res.Error = fmt.Errorf("cd failed: %w", err)
			res.Action = "failed"
			results = append(results, res)
			continue
		}
		time.Sleep(200 * time.Millisecond)

		if err := SendKeys(agent.Name, claudeCmd); err != nil {
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
func ConfigureAgentsAsync(cfg *config.FleetConfig) {
	logDir := filepath.Join(config.FleetDir(), "logs")
	os.MkdirAll(logDir, 0755)
	rotateConfigLogs(logDir, 5)

	timestamp := time.Now().Format("20060102-150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("configure-%s.log", timestamp))
	scriptPath := filepath.Join(config.FleetDir(), "configure-agents.sh")

	relayURL := cfg.Project.RelayURL
	if relayURL == "" {
		relayURL = "http://localhost:8090/mcp"
	}

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
	for _, agent := range cfg.Agents {
		session := SessionName(agent.Name)
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
		script.WriteString(fmt.Sprintf("  tmux send-keys -t %s '%s' Enter\n", session, strings.ReplaceAll(registerCmd, "'", "'\\''")))
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

		if !agent.IsExecutive {
			script.WriteString(fmt.Sprintf("  wait_prompt %s 15\n", session))
			script.WriteString(fmt.Sprintf("  tmux send-keys -t %s '/relay talk' Enter\n", session))
		}

		script.WriteString(fmt.Sprintf("  echo '✓ %s configured'\n", session))
		script.WriteString("else\n")
		script.WriteString(fmt.Sprintf("  echo '⚠ %s: timeout'\n", session))
		script.WriteString("fi\n\n")
	}

	script.WriteString("echo 'all agents configured'\n")

	os.WriteFile(scriptPath, []byte(script.String()), 0755)

	// Launch as detached process — survives fleet exit
	cmd := exec.Command("bash", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Start()
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

// WakeAgent sends /relay talk to a sleeping agent via tmux.
// The talk loop will stop on its own after 3 empty checks.
func WakeAgent(agent string) error {
	if !HasSession(agent) {
		return fmt.Errorf("no tmux session for agent %q", agent)
	}
	return SendKeys(agent, "/relay talk")
}
