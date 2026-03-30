package runner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const sessionPrefix = "pm"

func SessionName(agent string) string {
	return sessionPrefix + "-" + agent
}

func buildCreateArgs(session, cwd string) []string {
	return []string{"new-session", "-d", "-s", session, "-c", cwd}
}

func buildSendKeysArgs(session, text string) []string {
	return []string{"send-keys", "-t", session, text, "Enter"}
}

func CreateSession(agent, cwd string) error {
	session := SessionName(agent)
	args := buildCreateArgs(session, cwd)
	return exec.Command("tmux", args...).Run()
}

func SendKeys(agent, text string) error {
	session := SessionName(agent)
	args := buildSendKeysArgs(session, text)
	return exec.Command("tmux", args...).Run()
}

func KillSession(agent string) error {
	session := SessionName(agent)
	return exec.Command("tmux", "kill-session", "-t", session).Run()
}

func HasSession(agent string) bool {
	session := SessionName(agent)
	err := exec.Command("tmux", "has-session", "-t", session).Run()
	return err == nil
}

func CapturePane(agent string) (string, error) {
	session := SessionName(agent)
	out, err := exec.Command("tmux", "capture-pane", "-t", session, "-p").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func WaitForPrompt(agent string, timeout time.Duration) error {
	session := SessionName(agent)
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

func IsIdle(agent string) bool {
	out, err := CapturePane(agent)
	if err != nil {
		return false
	}
	return strings.Contains(out, "❯")
}

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

func KillAllFleetSessions() error {
	sessions, _ := ListFleetSessions()
	for _, s := range sessions {
		exec.Command("tmux", "kill-session", "-t", s).Run()
	}
	return nil
}

type AgentStatus struct {
	Name    string
	HasTmux bool
	IsIdle  bool
}

// DetectConflicts checks which agents already have running tmux sessions.
func DetectConflicts(agents []string) []AgentStatus {
	var statuses []AgentStatus
	for _, name := range agents {
		s := AgentStatus{Name: name}
		s.HasTmux = HasSession(name)
		if s.HasTmux {
			s.IsIdle = IsIdle(name)
		}
		statuses = append(statuses, s)
	}
	return statuses
}
