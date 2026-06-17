package runner

import (
	"os/exec"
	"strings"
	"testing"
)

func fakeExec(t *testing.T, paneOutput string, calls *[][]string) func(string, ...string) *exec.Cmd {
	t.Helper()
	return func(name string, args ...string) *exec.Cmd {
		*calls = append(*calls, append([]string{name}, args...))
		if len(args) > 0 && args[0] == "capture-pane" {
			return exec.Command("printf", "%s", paneOutput)
		}
		return exec.Command("true") // send-keys → succeed silently
	}
}

func TestWakeSessionIfDormantWakesAtPrompt(t *testing.T) {
	var calls [][]string
	orig := execCommand
	execCommand = fakeExec(t, "some output\n❯ ", &calls)
	defer func() { execCommand = orig }()

	origSettle := submitSettle
	submitSettle = 0
	defer func() { submitSettle = origSettle }()

	woke, err := WakeSessionIfDormant("fleet-acme-dev", "dev", "acme")
	if err != nil || !woke {
		t.Fatalf("dormant pane must wake: woke=%v err=%v", woke, err)
	}
	// Pin the full ordered sequence: capture-pane probe, then the identity
	// preamble (send-keys + Enter) BEFORE /relay talk (typed alone), then the
	// separate Enter. This guards the preamble-then-talk ordering invariant — not
	// just that /relay talk was sent somewhere — mirroring
	// TestWakeAgentSendsPreambleThenTalk.
	want := [][]string{
		{"tmux", "capture-pane", "-t", "fleet-acme-dev", "-p"},
		{"tmux", "send-keys", "-t", "fleet-acme-dev", identityPreamble("dev", "acme"), "Enter"},
		{"tmux", "send-keys", "-t", "fleet-acme-dev", "/relay talk"},
		{"tmux", "send-keys", "-t", "fleet-acme-dev", "Enter"},
	}
	assertCalls(t, calls, want)
}

func TestWakeSessionIfDormantSkipsBusy(t *testing.T) {
	var calls [][]string
	orig := execCommand
	execCommand = fakeExec(t, "Claude is working...\n", &calls) // no ❯
	defer func() { execCommand = orig }()

	origSettle := submitSettle
	submitSettle = 0
	defer func() { submitSettle = origSettle }()

	woke, err := WakeSessionIfDormant("fleet-acme-dev", "dev", "acme")
	if err != nil || woke {
		t.Fatalf("busy pane must NOT wake: woke=%v err=%v", woke, err)
	}
	for _, c := range calls {
		if wcontains(c, "/relay talk") {
			t.Fatalf("must not send /relay talk to a busy pane; calls=%v", calls)
		}
	}
}

func TestWakeSessionIfDormantMissingSessionIsSkip(t *testing.T) {
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd { return exec.Command("false") } // capture-pane fails
	defer func() { execCommand = orig }()

	woke, err := WakeSessionIfDormant("fleet-acme-ghost", "ghost", "acme")
	if err != nil || woke {
		t.Fatalf("missing session must be (false, nil), got woke=%v err=%v", woke, err)
	}
}

func wcontains(ss []string, want string) bool {
	for _, s := range ss {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
