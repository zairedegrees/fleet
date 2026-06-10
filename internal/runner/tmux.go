package runner

import (
	"errors"
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

// buildTypeArgs types text into a pane WITHOUT submitting (no trailing Enter).
func buildTypeArgs(session, text string) []string {
	return []string{"send-keys", "-t", session, text}
}

// buildEnterArgs submits the current pane input by sending Enter alone.
func buildEnterArgs(session string) []string {
	return []string{"send-keys", "-t", session, "Enter"}
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

// classifyListErr maps a `tmux list-sessions` failure to either nil (the benign
// "no server running"/"no sessions" case — treat as zero sessions) or a real
// error when tmux is absent or failed unexpectedly.
func classifyListErr(err error, stderr []byte) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("tmux not found on PATH — install tmux (brew install tmux)")
	}
	s := strings.ToLower(string(stderr))
	if strings.Contains(s, "no server running") || strings.Contains(s, "no sessions") {
		return nil
	}
	return fmt.Errorf("tmux list-sessions failed: %s", strings.TrimSpace(string(stderr)))
}

// listSessions returns every tmux session name, distinguishing a broken/absent
// tmux (error) from a server with no sessions (empty, nil).
func listSessions() ([]string, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		var exitErr *exec.ExitError
		var stderr []byte
		if errors.As(err, &exitErr) {
			stderr = exitErr.Stderr
		}
		if cerr := classifyListErr(err, stderr); cerr != nil {
			return nil, cerr
		}
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// ListProjectSessions returns sessions for a specific project.
func ListProjectSessions(project string) ([]string, error) {
	prefix := sessionPrefix + "-" + sanitizeProject(project) + "-"
	all, err := listSessions()
	if err != nil {
		return nil, err
	}
	var sessions []string
	for _, line := range all {
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
	all, err := listSessions()
	if err != nil {
		return nil, err
	}
	var sessions []string
	for _, line := range all {
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
	return SubmitCommand(project, agent, "/relay talk")
}

// SubmitCommand types a Claude TUI command into a pane and submits it with a
// SEPARATE Enter after a short settle delay. A /relay skill command sent as
// "text Enter" in one send-keys is typed but not submitted — the Enter is
// swallowed by the skill autocomplete (confirmed for /relay register).
func SubmitCommand(project, agent, cmd string) error {
	session := SessionName(project, agent)
	if err := exec.Command("tmux", buildTypeArgs(session, cmd)...).Run(); err != nil {
		return err
	}
	time.Sleep(time.Second)
	return exec.Command("tmux", buildEnterArgs(session)...).Run()
}

// waitGone polls gone() up to attempts times (interval apart), returning true as
// soon as it reports the session is gone — replaces a blind fixed sleep.
func waitGone(gone func() bool, attempts int, interval time.Duration) bool {
	for i := 0; i < attempts; i++ {
		if gone() {
			return true
		}
		time.Sleep(interval)
	}
	return gone()
}

// WaitSessionGone waits up to timeout for an agent's tmux session to exit on its
// own (e.g. after /exit), returning true if it did.
func WaitSessionGone(project, agent string, timeout time.Duration) bool {
	const interval = 200 * time.Millisecond
	attempts := int(timeout / interval)
	if attempts < 1 {
		attempts = 1
	}
	return waitGone(func() bool { return !HasSession(project, agent) }, attempts, interval)
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
