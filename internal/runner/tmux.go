package runner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const sessionPrefix = "fleet"

// sanitizeProject replaces characters that are invalid in tmux session names.
func sanitizeProject(project string) string {
	return strings.ReplaceAll(project, ".", "-")
}

func SessionName(project, agent string) string {
	return sessionPrefix + "-" + sanitizeProject(project) + "-" + agent
}

func buildCreateArgs(session, cwd string) []string {
	return []string{"new-session", "-d", "-s", session, "-c", cwd}
}

func buildSendKeysArgs(session, text string) []string {
	return []string{"send-keys", "-t", session, text, "Enter"}
}

func CreateSession(project, agent, cwd string) error {
	session := SessionName(project, agent)
	args := buildCreateArgs(session, cwd)
	return exec.Command("tmux", args...).Run()
}

func SendKeys(project, agent, text string) error {
	session := SessionName(project, agent)
	args := buildSendKeysArgs(session, text)
	return exec.Command("tmux", args...).Run()
}

func KillSession(project, agent string) error {
	session := SessionName(project, agent)
	return exec.Command("tmux", "kill-session", "-t", session).Run()
}

func HasSession(project, agent string) bool {
	session := SessionName(project, agent)
	err := exec.Command("tmux", "has-session", "-t", session).Run()
	return err == nil
}

func CapturePane(project, agent string) (string, error) {
	session := SessionName(project, agent)
	out, err := exec.Command("tmux", "capture-pane", "-t", session, "-p").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func WaitForPrompt(project, agent string, timeout time.Duration) error {
	session := SessionName(project, agent)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := exec.Command("tmux", "capture-pane", "-t", session, "-p").Output()
		if err == nil && strings.Contains(string(out), "❯") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for prompt on %s", session)
}

func IsIdle(project, agent string) bool {
	out, err := CapturePane(project, agent)
	if err != nil {
		return false
	}
	return strings.Contains(out, "❯")
}

// ListProjectSessions returns sessions for a specific project.
func ListProjectSessions(project string) ([]string, error) {
	prefix := sessionPrefix + "-" + sanitizeProject(project) + "-"
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil, nil
	}
	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, prefix) {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// AgentFromSession extracts the agent name from a session name.
func AgentFromSession(project, session string) string {
	prefix := sessionPrefix + "-" + sanitizeProject(project) + "-"
	return strings.TrimPrefix(session, prefix)
}

// ListFleetSessions returns ALL fleet sessions across all projects.
func ListFleetSessions() ([]string, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil, nil
	}
	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, sessionPrefix+"-") {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// KillProjectSessions kills all sessions for a specific project.
func KillProjectSessions(project string) (int, error) {
	sessions, _ := ListProjectSessions(project)
	for _, s := range sessions {
		exec.Command("tmux", "kill-session", "-t", s).Run()
	}
	return len(sessions), nil
}

// KillAllFleetSessions kills ALL fleet sessions.
func KillAllFleetSessions() error {
	sessions, _ := ListFleetSessions()
	for _, s := range sessions {
		exec.Command("tmux", "kill-session", "-t", s).Run()
	}
	return nil
}

// WakeAgent sends /relay talk to a running agent session.
func WakeAgent(project, agent string) error {
	if !HasSession(project, agent) {
		return fmt.Errorf("no tmux session for agent %q in project %q", agent, project)
	}
	return SendKeys(project, agent, "/relay talk")
}

// DetectConflicts checks if any of the given agents already have sessions in the project.
func DetectConflicts(project string, agents []string) []string {
	var conflicts []string
	for _, agent := range agents {
		if HasSession(project, agent) {
			conflicts = append(conflicts, agent)
		}
	}
	return conflicts
}
