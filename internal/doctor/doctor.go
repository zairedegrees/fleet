package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Check struct {
	Name   string
	Status string // "ok", "missing", "error"
	Detail string
	FixCmd string
}

func Run() []Check {
	return []Check{
		checkTmux(),
		checkClaude(),
		checkRelay(),
		checkITerm2(),
	}
}

func checkTmux() Check {
	c := Check{Name: "tmux"}
	out, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		c.Status = "missing"
		c.Detail = "tmux not installed"
		c.FixCmd = "brew install tmux"
		return c
	}
	c.Status = "ok"
	c.Detail = strings.TrimSpace(string(out))
	return c
}

func checkClaude() Check {
	c := Check{Name: "claude"}
	out, err := exec.Command("claude", "--version").Output()
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

func checkRelay() Check {
	c := Check{Name: "wrai.th relay"}
	_, err := exec.Command("curl", "-s", "-f", "http://localhost:8090/mcp").Output()
	if err != nil {
		c.Status = "error"
		c.Detail = "relay not reachable at localhost:8090"
		c.FixCmd = "Start wrai.th relay server"
		return c
	}
	c.Status = "ok"
	c.Detail = "http://localhost:8090"
	return c
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
	fmt.Println("\n  Fleet Doctor\n")
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
