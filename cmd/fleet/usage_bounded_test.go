package main

import (
	"strings"
	"testing"
)

func TestRenderUsageShowsBoundedEstimate(t *testing.T) {
	out := renderUsage([]projectUsage{{Project: "p", RelayURL: "x", Agents: 2, Bounded: 1, ProjBoundedUSD: 3.0, Registered: -1, Active: -1}})
	if !strings.Contains(out, "bounded") || !strings.Contains(out, "$3.00") {
		t.Fatalf("usage must show bounded estimate: %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "est") { // labelled as estimate
		t.Fatalf("must be labelled an estimate: %q", out)
	}
}

func TestRenderUsageOmitsBoundedWhenNone(t *testing.T) {
	out := renderUsage([]projectUsage{{Project: "p", RelayURL: "x", Agents: 2, Bounded: 0, Registered: -1, Active: -1}})
	if strings.Contains(out, "bounded (est)") {
		t.Fatalf("no bounded line when zero: %q", out)
	}
}
