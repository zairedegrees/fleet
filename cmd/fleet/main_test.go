package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

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
