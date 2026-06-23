package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/supervisor"
)

// demoAgent is one simulated agent. working drives the real deriveOpState
// (active + a task → "working", active + none → "idle"); lastSeen feeds the real
// relativeTime so "seen Xs ago" advances against the wall clock between frames.
// posture ("" falls back to autoTalk) lets the demo show a bounded agent.
type demoAgent struct {
	name     string
	autoTalk bool
	posture  string
	working  bool
	lastSeen time.Time
}

type demoModel struct {
	project string
	agents  []demoAgent
}

// newDemoModel seeds a five-agent fleet with staggered last-seen times so the
// first frame already looks like a real, lived-in team. now is injected so the
// model is deterministic under test.
func newDemoModel(now time.Time) demoModel {
	return demoModel{
		project: "acme-api",
		agents: []demoAgent{
			{name: "architect", autoTalk: true, lastSeen: now.Add(-8 * time.Second)},
			{name: "builder", autoTalk: false, lastSeen: now.Add(-3 * time.Second)},
			{name: "auditor", posture: config.PostureBounded, lastSeen: now.Add(-47 * time.Second)},
			{name: "scribe", autoTalk: false, lastSeen: now.Add(-2 * time.Minute)},
			{name: "lead", autoTalk: true, lastSeen: now.Add(-19 * time.Second)},
		},
	}
}

// advanceDemo mutates the model one frame forward along a looping 12-step
// scenario: a task lands on builder (idle → working → idle), auditor checks in
// mid-cycle, and the lead picks up a follow-up — so the view always shows
// motion. Pure in (step mod cycle, now), hence deterministically testable.
// Agent indices: 0 architect, 1 builder, 2 auditor, 3 scribe, 4 lead.
func advanceDemo(m *demoModel, step int, now time.Time) {
	switch step % 12 {
	case 3: // dispatch lands on builder
		m.agents[1].working = true
		m.agents[1].lastSeen = now
	case 5: // auditor checks in
		m.agents[2].lastSeen = now
	case 7: // builder finishes
		m.agents[1].working = false
		m.agents[1].lastSeen = now
	case 8: // follow-up lands on the lead
		m.agents[4].working = true
		m.agents[4].lastSeen = now
	case 11: // lead finishes; cycle resets
		m.agents[4].working = false
		m.agents[4].lastSeen = now
	}
}

// demoProjects maps the simulated model onto the exact projectStatus shape the
// real renderer consumes, so fleet --demo exercises renderStatus / agentLine /
// deriveOpState / relativeTime unchanged.
func demoProjects(m demoModel) []projectStatus {
	agents := make([]agentStatus, 0, len(m.agents))
	bounded := 0
	for _, a := range m.agents {
		tasks := 0
		if a.working {
			tasks = 1
		}
		st := agentStatus{
			Agent:      a.name,
			RelayState: "active",
			Tasks:      tasks,
			HasSession: true,
			AutoTalk:   a.autoTalk,
			Posture:    a.posture,
			LastSeen:   a.lastSeen.Format(time.RFC3339),
		}
		if a.posture == config.PostureBounded {
			bounded++
			st.bounded = &supervisor.AgentState{WakesToday: 12, SpentUSD: 0.72}
			st.boundedPolicy = config.BoundedPolicy{MaxWakesPerDay: 50, BudgetUSD: 3.00}
		}
		agents = append(agents, st)
	}
	ps := projectStatus{Project: m.project, Agents: agents, BoundedAgents: bounded}
	if bounded > 0 {
		ps.SupervisorRunning = true
		ps.SupervisorPID = 4823
	}
	return []projectStatus{ps}
}

// demoBanner heads the simulated status view so nobody mistakes it for a real
// fleet.
const demoBanner = "  DEMO — simulated fleet, no agents are running. Run `fleet` for the real thing.\n\n"

// demoRenderer returns the render function fleet --demo feeds to watchStatus.
// It owns the scenario step counter and model, so each call advances the fleet
// one frame and returns a fully rendered screen (banner + real status view).
func demoRenderer() func() string {
	model := newDemoModel(time.Now())
	step := 0
	return func() string {
		now := time.Now()
		advanceDemo(&model, step, now)
		step++
		return demoBanner + renderStatus(demoProjects(model), len(model.agents), "", now)
	}
}

// runDemo plays a scripted, in-memory fleet through the real status --watch
// renderer until ctrl+c. No tmux, relay, Claude, or persistence is touched, so
// it runs anywhere with zero prerequisites. It reuses the same (tested)
// watchStatus loop as fleet --status --watch.
func runDemo() error {
	interval := flagInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	render := demoRenderer()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	watchStatus(os.Stdout, interval, render, ticker.C, sig)
	return nil
}
