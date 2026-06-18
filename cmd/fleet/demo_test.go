package main

import (
	"strings"
	"testing"
	"time"
)

func TestNewDemoModelSeedsFiveStaggeredAgents(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	m := newDemoModel(now)

	if m.project == "" {
		t.Fatal("demo model has no project name")
	}
	if len(m.agents) != 5 {
		t.Fatalf("got %d agents, want 5", len(m.agents))
	}
	wantNames := []string{"architect", "builder", "auditor", "scribe", "lead"}
	for i, want := range wantNames {
		if m.agents[i].name != want {
			t.Errorf("agent[%d] = %q, want %q", i, m.agents[i].name, want)
		}
	}
	for _, a := range m.agents {
		if !a.lastSeen.Before(now) {
			t.Errorf("agent %q lastSeen %v is not before now %v", a.name, a.lastSeen, now)
		}
		if a.working {
			t.Errorf("agent %q starts working; want idle on the first frame", a.name)
		}
	}
}

func TestAdvanceDemoBuilderWorkCycle(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	m := newDemoModel(now)

	advanceDemo(&m, 0, now)
	if m.agents[1].working {
		t.Fatal("builder working at step 0; want idle")
	}

	advanceDemo(&m, 3, now) // dispatch lands on builder
	if !m.agents[1].working {
		t.Fatal("builder not working at step 3; want working")
	}
	if !m.agents[1].lastSeen.Equal(now) {
		t.Errorf("builder lastSeen not reset on dispatch: %v", m.agents[1].lastSeen)
	}

	advanceDemo(&m, 7, now) // builder finishes
	if m.agents[1].working {
		t.Fatal("builder still working at step 7; want idle")
	}
}

func TestDemoProjectsRenderWorkingAndIdle(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	m := newDemoModel(now)
	advanceDemo(&m, 3, now) // builder working, the rest idle

	out := renderStatus(demoProjects(m, now), len(m.agents), "", now)

	if !strings.Contains(out, "builder") || !strings.Contains(out, "working") {
		t.Errorf("expected a working builder in:\n%s", out)
	}
	if !strings.Contains(out, "idle") {
		t.Errorf("expected at least one idle agent in:\n%s", out)
	}
	if !strings.Contains(out, "seen ") {
		t.Errorf("expected last-seen segments in:\n%s", out)
	}
	if !strings.Contains(out, "auto-talk") || !strings.Contains(out, "on-demand") {
		t.Errorf("expected both posture labels in:\n%s", out)
	}
}

func TestDemoRendererBannerAndProgress(t *testing.T) {
	render := demoRenderer()

	first := render()
	if !strings.Contains(first, "DEMO —") {
		t.Errorf("first frame missing demo banner:\n%s", first)
	}

	sawWorking := strings.Contains(first, "working")
	for i := 0; i < 12 && !sawWorking; i++ {
		if strings.Contains(render(), "working") {
			sawWorking = true
		}
	}
	if !sawWorking {
		t.Error("scenario never produced a working agent across a full cycle")
	}
}
