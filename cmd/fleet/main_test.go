package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/runner"
)

// `fleet --kill` must NEVER fall through to killing every project's sessions
// just because the last config could not be loaded (missing/corrupt last.toml,
// first run). That escalation has a huge blast radius in a multi-project setup.
func TestRunKillDoesNotEscalateWhenNoLastConfig(t *testing.T) {
	orig := loadLastConfig
	t.Cleanup(func() { loadLastConfig = orig })
	loadLastConfig = func() (*config.FleetConfig, error) {
		return nil, errors.New("no last config")
	}

	err := runKill()
	if err == nil {
		t.Fatal("expected a guidance error, got nil — did --kill escalate to --kill-all?")
	}
	if !strings.Contains(err.Error(), "kill-all") {
		t.Errorf("error should point the user to --kill-all, got: %v", err)
	}
}

// A partial launch must NOT be reported as success: every failed agent's error
// is surfaced and a non-nil error is returned so the CLI exits non-zero.
func TestReportLaunchResultsErrorsOnFailure(t *testing.T) {
	results := []runner.LaunchResult{
		{Agent: "dev", Success: true, Action: "created"},
		{Agent: "ops", Success: false, Action: "failed", Error: errors.New("tmux create failed: boom")},
	}
	var buf bytes.Buffer

	err := reportLaunchResults(&buf, results)
	if err == nil {
		t.Fatal("expected an error when an agent failed, got nil")
	}
	out := buf.String()
	if !strings.Contains(out, "ops") || !strings.Contains(out, "boom") {
		t.Errorf("failure output must name the failed agent and its error, got: %q", out)
	}
}

// `fleet add` must validate the agent (reusing config.Validate) BEFORE creating
// any tmux session, so an invalid name/role can't produce a half-broken agent.
func TestRunAddRejectsInvalidAgentBeforeTouchingTmux(t *testing.T) {
	orig := loadLastConfig
	t.Cleanup(func() { loadLastConfig = orig })
	loadLastConfig = func() (*config.FleetConfig, error) {
		return &config.FleetConfig{
			Project: config.ProjectConfig{Name: "proj", Cwd: "/tmp"},
			Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
		}, nil
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("role", "", "")
	cmd.Flags().String("color", "green", "")
	cmd.Flags().String("reports-to", "", "")
	cmd.Flags().Set("name", "bad name!")
	cmd.Flags().Set("role", "Dev")

	err := runAdd(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid agent name") {
		t.Errorf("expected validation to reject the invalid name, got: %v", err)
	}
}

func TestTailLines(t *testing.T) {
	if got := tailLines("a\nb\nc\nd", 2); got != "c\nd" {
		t.Errorf("tailLines should keep the last n lines, got %q", got)
	}
	if got := tailLines("a\nb", 5); got != "a\nb" {
		t.Errorf("tailLines should keep everything when shorter than n, got %q", got)
	}
}

// Real capture-pane output ends with a newline; the trailing empty element
// after the split must not eat one of the requested lines (-n 50 showed 49).
func TestTailLinesTrailingNewlineKeepsFullCount(t *testing.T) {
	if got := tailLines("a\nb\nc\n", 2); got != "b\nc" {
		t.Errorf("want the last 2 real lines \"b\\nc\", got %q", got)
	}
	if got := tailLines("a\nb\nc\n", 3); got != "a\nb\nc" {
		t.Errorf("want all 3 real lines, got %q", got)
	}
}

// `fleet logs -n -5 dev` reached tailLines with a negative n and panicked on
// the slice bound; non-positive n must yield no lines instead.
func TestTailLinesClampsNonPositiveN(t *testing.T) {
	if got := tailLines("a\nb\nc", -5); got != "" {
		t.Errorf("negative n must yield no lines, got %q", got)
	}
	if got := tailLines("a\nb\nc", 0); got != "" {
		t.Errorf("zero n must yield no lines, got %q", got)
	}
}

// The flag itself must be rejected up front with a clear error, before any
// config/tmux work.
func TestRunLogsRejectsNonPositiveLines(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().IntP("lines", "n", 50, "")
	cmd.Flags().BoolP("follow", "f", true, "")
	cmd.Flags().Set("lines", "-5")

	err := runLogs(cmd, []string{"dev"})
	if err == nil || !strings.Contains(err.Error(), "--lines") {
		t.Errorf("expected a --lines flag error, got: %v", err)
	}
}

// `fleet logs -f` must tell the user how to get out.
func TestLogsHeaderHasCtrlCHint(t *testing.T) {
	h := logsHeader("proj", "dev")
	if !strings.Contains(h, "Ctrl-C") {
		t.Errorf("follow header must hint Ctrl-C, got %q", h)
	}
	if !strings.Contains(h, "proj") || !strings.Contains(h, "dev") {
		t.Errorf("follow header must name the project and agent, got %q", h)
	}
}

// When the agent's tmux session dies mid-follow, `logs -f` must exit with a
// non-nil error naming the agent — not silently succeed with exit code 0.
func TestFollowPaneErrorsWhenSessionDies(t *testing.T) {
	var buf bytes.Buffer
	capture := func() (string, error) {
		return "", errors.New("no such session")
	}

	err := followPane(&buf, capture, "dev", logsHeader("proj", "dev"), "old", 50, time.Millisecond)
	if err == nil {
		t.Fatal("expected a non-nil error when the tmux session dies, got nil")
	}
	if !strings.Contains(err.Error(), "dev") {
		t.Errorf("error must name the agent, got: %v", err)
	}
}

// Refreshes must reposition the cursor (\033[H) and erase leftovers (\033[K
// per line, \033[J below the frame) instead of clearing the whole screen
// (\033[2J), which flickers on every poll. The full clear happens exactly
// once, before the first frame.
func TestFollowPaneRefreshesWithoutFullClear(t *testing.T) {
	var buf bytes.Buffer
	calls := 0
	capture := func() (string, error) {
		calls++
		if calls == 1 {
			return "new output", nil
		}
		return "", errors.New("no such session")
	}

	followPane(&buf, capture, "dev", logsHeader("proj", "dev"), "old", 50, time.Millisecond)
	out := buf.String()
	if n := strings.Count(out, "\033[2J"); n != 1 {
		t.Errorf("full clear must happen exactly once, before the first frame, got %d in %q", n, out)
	}
	home := strings.LastIndex(out, "\033[H")
	if home < 0 {
		t.Fatalf("refresh must home the cursor, got %q", out)
	}
	refresh := out[home:]
	if strings.Contains(refresh, "\033[2J") {
		t.Errorf("refresh must not clear the full screen, got %q", refresh)
	}
	if !strings.Contains(refresh, "\033[K") {
		t.Errorf("refresh must erase each redrawn line's tail, got %q", refresh)
	}
	if !strings.HasSuffix(refresh, "\033[J") {
		t.Errorf("refresh must erase below the frame, got %q", refresh)
	}
	if !strings.Contains(refresh, "Ctrl-C") || !strings.Contains(refresh, "new output") {
		t.Errorf("refresh must redraw the header and the new content, got %q", refresh)
	}
}

// The initial followed frame must start from a cleared screen at the top-left
// so it shares the same origin as every \033[H refresh — otherwise the first
// refresh interleaves with the shell scrollback above the initial frame.
func TestFollowPaneClearsOnceBeforeInitialFrame(t *testing.T) {
	var buf bytes.Buffer
	capture := func() (string, error) {
		return "", errors.New("no such session")
	}

	followPane(&buf, capture, "dev", logsHeader("proj", "dev"), "initial frame", 50, time.Millisecond)
	out := buf.String()
	if !strings.HasPrefix(out, "\033[2J\033[H") {
		t.Errorf("followed output must start with a one-time clear+home, got %q", out)
	}
	if !strings.Contains(out, "initial frame") {
		t.Errorf("followPane must draw the initial frame itself, got %q", out)
	}
}

// A redraw shorter than the previous frame must be followed by an erase —
// \033[H alone leaves the previous frame's tail on screen (e.g. a spinner
// line redrawn as a bare prompt keeps "… 42s · 1234 tokens" visible).
func TestFollowPaneErasesStaleCharsOnShorterFrame(t *testing.T) {
	var buf bytes.Buffer
	calls := 0
	capture := func() (string, error) {
		calls++
		switch calls {
		case 1:
			return "Thinking… 42s · 1234 tokens", nil
		case 2:
			return "❯", nil
		}
		return "", errors.New("no such session")
	}

	followPane(&buf, capture, "dev", logsHeader("proj", "dev"), "old", 50, time.Millisecond)
	out := buf.String()
	idx := strings.LastIndex(out, "❯")
	if idx < 0 {
		t.Fatalf("expected the shorter frame to be drawn, got %q", out)
	}
	tail := out[idx+len("❯"):]
	if !strings.Contains(tail, "\033[J") && !strings.Contains(tail, "\033[K") {
		t.Errorf("shorter redraw must end with an erase sequence, got tail %q in %q", tail, out)
	}
}

// --relay-url must be the highest-priority source, then the config's URL,
// then the built-in default — the flag was declared since v1.0 but never wired.
func TestResolveRelayURLPriority(t *testing.T) {
	if got := resolveRelayURL("http://flag/mcp", "http://cfg/mcp"); got != "http://flag/mcp" {
		t.Errorf("flag must beat config URL, got %q", got)
	}
	if got := resolveRelayURL("", "http://cfg/mcp"); got != "http://cfg/mcp" {
		t.Errorf("config URL must beat default, got %q", got)
	}
	if got := resolveRelayURL("", ""); got != defaultRelayURL {
		t.Errorf("empty everything must fall back to default, got %q", got)
	}
	if got := resolveRelayURL("   ", "http://cfg/mcp"); got != "http://cfg/mcp" {
		t.Errorf("whitespace-only flag means unset and must not win the chain, got %q", got)
	}
	if got := resolveRelayURL("   ", ""); got != defaultRelayURL {
		t.Errorf("whitespace-only flag with no config must fall back to default, got %q", got)
	}
	if got := resolveRelayURL("  http://flag/mcp  ", "http://cfg/mcp"); got != "http://flag/mcp" {
		t.Errorf("flag value must be trimmed, got %q", got)
	}
}

func installFakeSessions(t *testing.T, sessions []string) *int {
	t.Helper()
	origList, origKill := listFleetSessions, killAllFleetSessions
	t.Cleanup(func() { listFleetSessions, killAllFleetSessions = origList, origKill })
	listFleetSessions = func() ([]string, error) { return sessions, nil }
	kills := 0
	killAllFleetSessions = func() error { kills++; return nil }
	return &kills
}

// --kill-all without --force must ask for y/N confirmation and abort on "n" —
// it kills every project's sessions, the audit demanded confirm-by-default.
// An abort is NOT a success: it must exit non-zero with an actionable stderr
// line, so a script can tell "killed" from "did nothing".
func TestKillAllPromptsAndAbortsOnNo(t *testing.T) {
	kills := installFakeSessions(t, []string{"fleet-a-dev", "fleet-b-dev"})

	var out, errOut bytes.Buffer
	if err := killAll(strings.NewReader("n\n"), &out, &errOut, false); err == nil {
		t.Fatal("aborting must surface as a non-nil error (non-zero exit), got nil")
	}
	if *kills != 0 {
		t.Error("answering n must not kill anything")
	}
	if !strings.Contains(out.String(), "[y/N]") {
		t.Errorf("expected a y/N prompt, got: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "Aborted") {
		t.Errorf("expected an explicit abort message on stderr, got: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "--force") {
		t.Errorf("abort message must point non-interactive callers to --force, got: %q", errOut.String())
	}
}

func TestKillAllProceedsOnYes(t *testing.T) {
	kills := installFakeSessions(t, []string{"fleet-a-dev"})

	var out, errOut bytes.Buffer
	if err := killAll(strings.NewReader("y\n"), &out, &errOut, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *kills != 1 {
		t.Error("answering y must kill the sessions")
	}
	if !strings.Contains(out.String(), "Killed 1 fleet session(s)") {
		t.Errorf("expected kill report, got: %q", out.String())
	}
}

// EOF defaults to No — a closed stdin (script, CI) must never nuke everything,
// but it must also never exit 0 pretending it did: the caller gets a non-zero
// exit and a stderr line pointing to --force.
func TestKillAllEOFDefaultsToAbort(t *testing.T) {
	kills := installFakeSessions(t, []string{"fleet-a-dev"})

	var out, errOut bytes.Buffer
	if err := killAll(strings.NewReader(""), &out, &errOut, false); err == nil {
		t.Fatal("EOF abort must surface as a non-nil error (non-zero exit), got nil")
	}
	if *kills != 0 {
		t.Error("EOF on stdin must abort, not kill")
	}
	if !strings.Contains(errOut.String(), "--force") {
		t.Errorf("EOF abort must tell non-interactive callers about --force on stderr, got: %q", errOut.String())
	}
}

func TestKillAllForceSkipsPrompt(t *testing.T) {
	kills := installFakeSessions(t, []string{"fleet-a-dev"})

	var out, errOut bytes.Buffer
	// No stdin available — --force must not read it.
	if err := killAll(strings.NewReader(""), &out, &errOut, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *kills != 1 {
		t.Error("--force must kill without confirmation")
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("--force must not prompt, got: %q", out.String())
	}
}

func TestKillAllNoSessionsNoPrompt(t *testing.T) {
	kills := installFakeSessions(t, nil)

	var out, errOut bytes.Buffer
	if err := killAll(strings.NewReader(""), &out, &errOut, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *kills != 0 {
		t.Error("nothing to kill must not call the killer")
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("nothing to kill must not prompt, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "No fleet sessions running") {
		t.Errorf("expected the empty report, got: %q", out.String())
	}
}

func TestReportLaunchResultsSilentOnAllSuccess(t *testing.T) {
	results := []runner.LaunchResult{
		{Agent: "dev", Success: true, Action: "created"},
		{Agent: "ops", Success: true, Action: "skipped"},
	}
	var buf bytes.Buffer

	if err := reportLaunchResults(&buf, results); err != nil {
		t.Errorf("expected no error when all agents succeeded, got: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no failure output when all succeeded, got: %q", buf.String())
	}
}
