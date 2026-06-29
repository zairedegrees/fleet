package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/cost"
	"github.com/zairedegrees/fleet/internal/relay"
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

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 6, 29, 14, 30, 0, 0, time.UTC)

	midnight, label, err := parseSince("today", now)
	if err != nil || !midnight.Equal(time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("today => %v (%q) err=%v", midnight, label, err)
	}
	if empty, _, _ := parseSince("", now); !empty.Equal(time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)) {
		t.Error("empty --since must default to today")
	}
	if z, _, _ := parseSince("all", now); !z.IsZero() {
		t.Error("all must be the zero time (no lower bound)")
	}
	if d, _, err := parseSince("2h", now); err != nil || !d.Equal(now.Add(-2*time.Hour)) {
		t.Errorf("2h => %v err=%v", d, err)
	}
	if _, _, err := parseSince("bogus", now); err == nil {
		t.Error("bogus --since must error")
	}
}

func TestBuildCostMeasuresAndFlagsUnknown(t *testing.T) {
	projectsDir := t.TempDir()
	sub := filepath.Join(projectsDir, "-proj")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// dev: 1M input on opus = $5.00 exactly
	os.WriteFile(filepath.Join(sub, "sess-dev.jsonl"), []byte(
		`{"type":"assistant","timestamp":"2026-06-29T10:00:00Z","message":{"model":"claude-opus-4-8","usage":{"input_tokens":1000000,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`+"\n"), 0o644)

	fake := &fakeQuerier{agents: map[string][]relay.Agent{"demo": {
		{Name: "dev", SessionID: "sess-dev"},
		{Name: "ghost"}, // no session_id → unknown
	}}}
	installFakeRelay(t, fake)

	pcs := buildCost([]*config.FleetConfig{usageConfig("demo")}, "", defaultRelayURL, time.Time{}, "all", projectsDir)
	if len(pcs) != 1 {
		t.Fatalf("want 1 project, got %d", len(pcs))
	}
	p := pcs[0]

	var dev *agentCost
	for i := range p.Agents {
		if p.Agents[i].Name == "dev" {
			dev = &p.Agents[i]
		}
	}
	if dev == nil || !dev.USDKnown || dev.USD < 4.99 || dev.USD > 5.01 {
		t.Errorf("dev measured spend wrong: %+v", dev)
	}
	if p.TotalKnown {
		t.Error("ghost agent has no session_id → project total must be unknown")
	}
}

func TestBuildCostRelayDownIsExplicit(t *testing.T) {
	fake := &fakeQuerier{listErr: errTestRelayDown}
	installFakeRelay(t, fake)
	pcs := buildCost([]*config.FleetConfig{usageConfig("demo")}, "", defaultRelayURL, time.Time{}, "all", t.TempDir())
	if len(pcs) != 1 || pcs[0].RelayWarning == "" || pcs[0].TotalKnown {
		t.Errorf("relay down must set a warning and unknown total: %+v", pcs)
	}
}

var errTestRelayDown = fmt.Errorf("connection refused")
