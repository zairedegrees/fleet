package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/zairedegrees/fleet/internal/relay"
	"github.com/zairedegrees/fleet/internal/relaymgr"
)

// execCommand is the seam tests swap to intercept external probes.
var execCommand = exec.Command

type Check struct {
	Name   string
	Status string // "ok", "missing", "error"
	Detail string
	FixCmd string
}

func Run(relayURL string) []Check {
	return run(relayURL, runtime.GOOS)
}

func run(relayURL, goos string) []Check {
	checks := []Check{
		checkTmux(goos),
		checkClaude(),
		checkRelay(relayURL),
	}
	if goos == "darwin" {
		checks = append(checks, checkITerm2())
	}
	return checks
}

func installHint(goos, pkg string) string {
	switch goos {
	case "darwin":
		return "brew install " + pkg
	case "linux":
		return "sudo apt install " + pkg
	default:
		return "install " + pkg + " with your system package manager"
	}
}

func checkTmux(goos string) Check {
	out, err := execCommand("tmux", "-V").Output()
	return tmuxCheck(goos, strings.TrimSpace(string(out)), err)
}

// tmuxCheck builds the tmux check result from the probe outcome; pure so the
// goos → installHint wiring is testable without touching PATH.
func tmuxCheck(goos, version string, probeErr error) Check {
	c := Check{Name: "tmux"}
	if probeErr != nil {
		c.Status = "missing"
		c.Detail = "tmux not installed"
		c.FixCmd = installHint(goos, "tmux")
		return c
	}
	c.Status = "ok"
	c.Detail = version
	return c
}

func checkClaude() Check {
	c := Check{Name: "claude"}
	out, err := execCommand("claude", "--version").Output()
	if err != nil {
		c.Status = "missing"
		c.Detail = "Claude Code CLI not installed"
		c.FixCmd = "npm install -g @anthropic-ai/claude-code"
		return c
	}
	c.Status = "ok"
	c.Detail = strings.TrimSpace(string(out))
	return c
}

// relayCheckFor builds the relay Check from reachability + whether fleet's
// managed agent-relay binary is present. With a fleet-managed relay the relay is
// never a blocking prerequisite, so every case is "ok" (valid Status values are
// "ok"/"missing"/"error") — only the Detail differs.
func relayCheckFor(reachable, binaryPresent bool) Check {
	c := Check{Name: "wrai.th relay", Status: "ok"}
	switch {
	case reachable:
		c.Detail = "reachable"
	case binaryPresent:
		c.Detail = "managed by fleet (auto-starts on launch)"
	default:
		c.Detail = "fleet will download & start it on first launch (with consent)"
	}
	return c
}

func checkRelay(relayURL string) Check {
	// The /mcp endpoint is an SSE stream: a bare GET never closes the body and
	// would hang forever. Probe it the way the rest of fleet does instead, with
	// a bounded JSON-RPC tools/call (Health), so a streaming relay can't block us.
	reachable := relay.NewClientWithTimeout(relayURL, 3*time.Second).Health() == nil
	_, statErr := os.Stat(relaymgr.BinPath())
	return relayCheckFor(reachable, statErr == nil)
}

func checkITerm2() Check {
	c := Check{Name: "iTerm2"}
	_, err := os.Stat("/Applications/iTerm.app")
	if err != nil {
		c.Status = "missing"
		c.Detail = "optional — fallback to tmux attach"
		c.FixCmd = "brew install --cask iterm2"
		return c
	}
	c.Status = "ok"
	c.Detail = "/Applications/iTerm.app"
	return c
}

func Print(checks []Check) {
	fmt.Printf("\n  Fleet Doctor\n\n")
	for _, c := range checks {
		icon := "✓"
		if c.Status == "missing" {
			icon = "✗"
		} else if c.Status == "error" {
			icon = "⚠"
		}
		fmt.Printf("  %s %-15s %s\n", icon, c.Name, c.Detail)
		if c.FixCmd != "" && c.Status != "ok" {
			fmt.Printf("    → %s\n", c.FixCmd)
		}
	}
	fmt.Println()
}
