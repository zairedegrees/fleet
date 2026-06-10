package runner

import (
	"os/exec"
	"reflect"
	"testing"
	"time"
)

// stubExec swaps the execCommand seam for a recorder that never runs tmux —
// every spawned process is replaced by /usr/bin/true. Returns the recorded
// argv list (command name included).
func stubExec(t *testing.T) *[][]string {
	t.Helper()
	calls := &[][]string{}
	orig := execCommand
	execCommand = func(name string, arg ...string) *exec.Cmd {
		*calls = append(*calls, append([]string{name}, arg...))
		return exec.Command("true")
	}
	t.Cleanup(func() { execCommand = orig })
	return calls
}

func assertCalls(t *testing.T, got [][]string, want [][]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestCreateSessionArgv(t *testing.T) {
	calls := stubExec(t)
	if err := CreateSession("proj", "dev", "/tmp/wd"); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	assertCalls(t, *calls, [][]string{
		{"tmux", "new-session", "-d", "-s", "fleet-proj-dev", "-c", "/tmp/wd"},
	})
}

func TestSendKeysArgv(t *testing.T) {
	calls := stubExec(t)
	if err := SendKeys("proj", "dev", "cd /tmp/wd"); err != nil {
		t.Fatalf("SendKeys failed: %v", err)
	}
	assertCalls(t, *calls, [][]string{
		{"tmux", "send-keys", "-t", "fleet-proj-dev", "cd /tmp/wd", "Enter"},
	})
}

// SubmitCommand must issue TWO send-keys: the text alone, then Enter alone.
// Combined into one, the skill autocomplete swallows the Enter and the
// command is typed but never submitted.
func TestSubmitCommandArgvSeparateEnter(t *testing.T) {
	calls := stubExec(t)
	origSettle := submitSettle
	submitSettle = time.Millisecond
	t.Cleanup(func() { submitSettle = origSettle })

	if err := SubmitCommand("proj", "dev", "/relay talk"); err != nil {
		t.Fatalf("SubmitCommand failed: %v", err)
	}
	assertCalls(t, *calls, [][]string{
		{"tmux", "send-keys", "-t", "fleet-proj-dev", "/relay talk"},
		{"tmux", "send-keys", "-t", "fleet-proj-dev", "Enter"},
	})
}

func TestCapturePaneArgv(t *testing.T) {
	calls := stubExec(t)
	if _, err := CapturePane("proj", "dev"); err != nil {
		t.Fatalf("CapturePane failed: %v", err)
	}
	assertCalls(t, *calls, [][]string{
		{"tmux", "capture-pane", "-t", "fleet-proj-dev", "-p"},
	})
}

func TestKillSessionArgv(t *testing.T) {
	calls := stubExec(t)
	if err := KillSession("proj", "dev"); err != nil {
		t.Fatalf("KillSession failed: %v", err)
	}
	assertCalls(t, *calls, [][]string{
		{"tmux", "kill-session", "-t", "fleet-proj-dev"},
	})
}

func TestHasSessionArgv(t *testing.T) {
	calls := stubExec(t)
	if !HasSession("proj", "dev") {
		t.Error("stubbed tmux exits 0 — HasSession should report true")
	}
	assertCalls(t, *calls, [][]string{
		{"tmux", "has-session", "-t", "fleet-proj-dev"},
	})
}

func TestListSessionsArgv(t *testing.T) {
	calls := stubExec(t)
	if _, err := listSessions(); err != nil {
		t.Fatalf("listSessions failed: %v", err)
	}
	assertCalls(t, *calls, [][]string{
		{"tmux", "list-sessions", "-F", "#{session_name}"},
	})
}

// The detached configure script must be spawned as `bash <script>` so it
// survives fleet's exit.
func TestSpawnDetachedArgv(t *testing.T) {
	calls := stubExec(t)
	if err := spawnDetached("/tmp/configure-agents.sh"); err != nil {
		t.Fatalf("spawnDetached failed: %v", err)
	}
	assertCalls(t, *calls, [][]string{
		{"bash", "/tmp/configure-agents.sh"},
	})
}
