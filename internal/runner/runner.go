package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nazaire/fleet/internal/config"
	"github.com/nazaire/fleet/internal/relay"
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

// ConfigureAgentsAsync configures agents in the background using Go goroutines.
// It waits for each agent's Claude prompt, then sends init commands.
// Logs output to ~/.fleet/logs/configure-{timestamp}.log
func ConfigureAgentsAsync(cfg *config.FleetConfig) {
	logDir := filepath.Join(config.FleetDir(), "logs")
	os.MkdirAll(logDir, 0755)

	timestamp := time.Now().Format("20060102-150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("configure-%s.log", timestamp))
	logFile, err := os.Create(logPath)
	if err != nil {
		fmt.Printf("  ⚠ Could not create log file: %v\n", err)
		logFile = nil
	}

	// Rotate: keep only last 5 log files
	rotateConfigLogs(logDir, 5)

	go func() {
		if logFile != nil {
			defer logFile.Close()
		}

		logMsg := func(format string, args ...interface{}) {
			msg := fmt.Sprintf(format, args...)
			if logFile != nil {
				fmt.Fprintln(logFile, msg)
			}
		}

		relayURL := cfg.Project.RelayURL
		if relayURL == "" {
			relayURL = "http://localhost:8090/mcp"
		}
		relayClient := relay.NewClient(relayURL)

		// Phase 1: Ensure profiles exist
		for _, agent := range cfg.Agents {
			relayClient.EnsureProfile(agent.Name, agent.Role, cfg.Project.Name)
			logMsg("profile ensured: %s", agent.Name)
		}

		// Phase 2: Configure each agent sequentially (must wait for prompts)
		for _, agent := range cfg.Agents {
			session := SessionName(agent.Name)

			if err := WaitForPrompt(agent.Name, 90*time.Second); err != nil {
				logMsg("ERROR: %s timeout waiting for prompt", agent.Name)
				fmt.Printf("  ⚠ %s: timeout waiting for prompt\n", agent.Name)
				continue
			}

			SendKeys(agent.Name, "/rename "+agent.Name)
			time.Sleep(2 * time.Second)

			SendKeys(agent.Name, "/color "+agent.Color)
			time.Sleep(2 * time.Second)

			registerCmd := fmt.Sprintf("/relay register %s %s %s",
				agent.Name, cfg.Project.Name, agent.Role)
			if agent.ReportsTo != "" {
				registerCmd += " Reports to " + agent.ReportsTo + "."
			}
			SendKeys(agent.Name, registerCmd)
			time.Sleep(3 * time.Second)

			// Vault injection
			vaultDir := filepath.Join(cfg.Project.Cwd, ".fleet", "vault")
			docs, err := config.ResolveVaultDocs(vaultDir, agent)
			if err != nil {
				logMsg("ERROR: vault resolve for %s: %v", agent.Name, err)
			} else if len(docs) > 0 {
				totalSize := config.VaultSize(docs)
				if totalSize > int64(config.VaultSizeWarningBytes) {
					logMsg("WARNING: vault for %s is %dKB (>50KB threshold)", agent.Name, totalSize/1024)
					fmt.Printf("  ⚠ %s: vault docs total %dKB (>50KB)\n", agent.Name, totalSize/1024)
				}
				for _, doc := range docs {
					if err := relayClient.PushVaultDoc(cfg.Project.Name, doc.Path, doc.Content); err != nil {
						logMsg("ERROR: vault push %s for %s: %v", doc.Path, agent.Name, err)
					} else {
						logMsg("vault pushed: %s -> %s", doc.Path, agent.Name)
					}
				}
				logMsg("vault injection complete for %s: %d docs, %d bytes", agent.Name, len(docs), totalSize)
			}

			if agent.AutoTalk && !agent.IsExecutive {
				WaitForPrompt(agent.Name, 15*time.Second)
				SendKeys(agent.Name, "/relay talk")
			}

			logMsg("configured: %s (%s)", session, agent.Role)
			fmt.Printf("  ✓ %s configured\n", session)
		}

		logMsg("all agents configured")
	}()
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
