package main

import (
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/cost"
)

func TestRenderCostMeasuredIdleAndUnknownRows(t *testing.T) {
	projects := []projectCost{{
		Project: "fleet", RelayURL: "http://127.0.0.1:8787",
		Window:   "since today (00:00 local)",
		TotalUSD: 3.42, TotalKnown: true,
		Agents: []agentCost{
			{Name: "dev", USD: 3.42, USDKnown: true, ByModel: map[string]cost.Usage{
				"claude-opus-4-8": {In: 1_200_000, Out: 89_000, CacheRead: 4_100_000, CacheCreate: 210_000}}},
			{Name: "idle", USD: 0, USDKnown: true, ByModel: map[string]cost.Usage{}},
			{Name: "ghost", Note: "(no session_id — agent never ran)"},
		},
	}}
	out := renderCost(projects)
	for _, want := range []string{
		"[fleet]", "since today", "dev", "opus-4-8", "$3.42", "[measured]",
		"idle", "$0.00", "ghost", "no session_id", "total",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n%s", want, out)
		}
	}
}

func TestRenderCostUnknownTotalIsNeverFaked(t *testing.T) {
	projects := []projectCost{{
		Project: "p", TotalKnown: false,
		Agents: []agentCost{{Name: "x", Note: "(no transcript yet)"}},
	}}
	out := renderCost(projects)
	if !strings.Contains(out, "$?") {
		t.Errorf("unknown total must render $?, not a number:\n%s", out)
	}
}

func TestRenderCostRelayWarning(t *testing.T) {
	out := renderCost([]projectCost{{Project: "p", RelayWarning: "relay unavailable", TotalKnown: false}})
	if !strings.Contains(out, "relay unavailable") || !strings.Contains(out, "?") {
		t.Errorf("relay-down project must render ? with the warning:\n%s", out)
	}
}

func TestRenderCostNoteSanitized(t *testing.T) {
	// Note contains an ANSI escape sequence; term.Sanitize must strip the ESC byte.
	maliciousNote := "\x1b[31mhack\x1b[0m"
	projects := []projectCost{{
		Project: "p",
		Agents:  []agentCost{{Name: "bad-agent", Note: maliciousNote}},
	}}
	out := renderCost(projects)
	if strings.Contains(out, "\x1b") {
		t.Errorf("renderCost must not emit raw ESC bytes from Note; got:\n%s", out)
	}
	if !strings.Contains(out, "hack") {
		t.Errorf("renderCost must keep printable chars from Note; got:\n%s", out)
	}
}
