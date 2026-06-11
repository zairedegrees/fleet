# Relay Autonomy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Launched agents run the relay `/relay talk` lifecycle unattended by default (scoped allowlist), with an opt-in wizard toggle for full `--dangerously-skip-permissions` autonomy.

**Architecture:** A new `runner.ProvisionRelayPermissions` writes a `mcp__agent-relay__*` allow rule into `<cwd>/.claude/settings.local.json` (non-destructive merge, mirroring `runner.ProvisionMCP`), wired into `launch()` via a seam. A `skipPerms` flag on the wizard model, toggled with `P` on the agents panel, sets `Config.Claude.Flags` — a field the launch path already honors.

**Tech Stack:** Go 1.26, Bubble Tea TUI, `encoding/json`, BurntSushi TOML (via `internal/config`).

**Branch:** `fix/agent-autonomy-permissions` (already created; spec at `docs/specs/2026-06-12-relay-autonomy-design.md`).

---

### Task 1: `ProvisionRelayPermissions` (relay allowlist, default)

**Files:**
- Create: `internal/runner/permissions.go`
- Test: `internal/runner/permissions_test.go`

The test helper `readJSON(t, path)` already exists in `internal/runner/mcpjson_test.go` (same package) — reuse it, do not redefine it.

- [ ] **Step 1: Write the failing tests**

Create `internal/runner/permissions_test.go`:

```go
package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func allowList(t *testing.T, m map[string]any) []any {
	t.Helper()
	perms, ok := m["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("no permissions object: %v", m)
	}
	allow, ok := perms["allow"].([]any)
	if !ok {
		t.Fatalf("no permissions.allow array: %v", perms)
	}
	return allow
}

func hasRule(allow []any, rule string) bool {
	for _, e := range allow {
		if s, ok := e.(string); ok && s == rule {
			return true
		}
	}
	return false
}

func TestProvisionRelayPermissionsCreatesFreshFile(t *testing.T) {
	dir := t.TempDir()
	if err := ProvisionRelayPermissions(dir); err != nil {
		t.Fatalf("ProvisionRelayPermissions: %v", err)
	}
	m := readJSON(t, filepath.Join(dir, ".claude", "settings.local.json"))
	if !hasRule(allowList(t, m), "mcp__agent-relay__*") {
		t.Errorf("fresh file missing relay allow rule: %v", m)
	}
}

func TestProvisionRelayPermissionsPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	existing := `{"statusLine":{"type":"command","command":"foo"},"permissions":{"allow":["Bash(ls)"]}}`
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(existing), 0644)

	if err := ProvisionRelayPermissions(dir); err != nil {
		t.Fatalf("ProvisionRelayPermissions: %v", err)
	}
	m := readJSON(t, filepath.Join(claudeDir, "settings.local.json"))
	if _, ok := m["statusLine"]; !ok {
		t.Error("merge dropped the existing statusLine key")
	}
	allow := allowList(t, m)
	if !hasRule(allow, "Bash(ls)") {
		t.Error("merge dropped the existing Bash(ls) allow entry")
	}
	if !hasRule(allow, "mcp__agent-relay__*") {
		t.Error("merge did not add the relay allow rule")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "settings.local.json.bak")); err != nil {
		t.Error("expected a settings.local.json.bak backup")
	}
}

func TestProvisionRelayPermissionsIdempotent(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := ProvisionRelayPermissions(dir); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	m := readJSON(t, filepath.Join(dir, ".claude", "settings.local.json"))
	count := 0
	for _, e := range allowList(t, m) {
		if s, ok := e.(string); ok && s == "mcp__agent-relay__*" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("rule must appear exactly once after two runs, got %d", count)
	}
}

func TestProvisionRelayPermissionsMalformedExistingIsNotClobbered(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	path := filepath.Join(claudeDir, "settings.local.json")
	os.WriteFile(path, []byte("{not json"), 0644)

	if err := ProvisionRelayPermissions(dir); err == nil {
		t.Fatal("expected an error on malformed existing settings.local.json")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{not json" {
		t.Error("malformed file must not be overwritten")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/runner/ -run TestProvisionRelayPermissions -v`
Expected: compile error / FAIL — `undefined: ProvisionRelayPermissions`.

- [ ] **Step 3: Write the implementation**

Create `internal/runner/permissions.go`:

```go
package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// relayPermissionRule is the Claude Code permission rule that pre-approves every
// agent-relay MCP tool, so a woken agent runs the relay lifecycle (claim/start/
// complete task, send_message, …) without an interactive prompt. The server
// segment must match the key ProvisionMCP writes into .mcp.json ("agent-relay").
const relayPermissionRule = "mcp__agent-relay__*"

// ProvisionRelayPermissions adds the relay allow-rule to the project's
// .claude/settings.local.json so launched agents reach the relay unattended
// without --dangerously-skip-permissions. It merges non-destructively: a fresh
// file is created when absent; an existing file is backed up (.bak) then merged,
// preserving every other key and every existing permissions.allow entry; the
// rule is added only if missing (idempotent). A malformed existing file is
// refused (error) rather than clobbered. settings.local.json is gitignored by
// fleet's .claude/.gitignore, so nothing lands in the user's committed tree.
func ProvisionRelayPermissions(projectCwd string) error {
	path := filepath.Join(projectCwd, ".claude", "settings.local.json")

	root := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &root); err != nil {
			return fmt.Errorf("existing %s is not valid JSON (left untouched): %w", path, err)
		}
		if err := os.WriteFile(path+".bak", b, 0644); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	perms, _ := root["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	allow, _ := perms["allow"].([]any)

	found := false
	for _, e := range allow {
		if s, ok := e.(string); ok && s == relayPermissionRule {
			found = true
			break
		}
	}
	if !found {
		allow = append(allow, relayPermissionRule)
	}
	perms["allow"] = allow
	root["permissions"] = perms

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.local.json: %w", err)
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/runner/ -run TestProvisionRelayPermissions -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/runner/permissions.go internal/runner/permissions_test.go
git commit -m "feat(runner): provision scoped agent-relay allowlist into settings.local.json"
```

---

### Task 2: Wire `ProvisionRelayPermissions` into `launch()`

**Files:**
- Modify: `cmd/fleet/main.go` (seam var block at lines 37-44; `launch()` after the `ProvisionMCP` call ~line 510)
- Modify: `cmd/fleet/launch_test.go` (`launchSeams` struct, `installLaunchSeams`, new test)

- [ ] **Step 1: Write the failing test**

In `cmd/fleet/launch_test.go`, add a field to the `launchSeams` struct (after `configuredURL string`):

```go
	permissionsCwd string
```

Add the seam to `installLaunchSeams`. Change the orig-capture line and cleanup to include `provisionPermissions`, and add the stub. The full updated function:

```go
func installLaunchSeams(t *testing.T) *launchSeams {
	t.Helper()
	s := &launchSeams{}
	origSave, origCreate, origGrid, origConfigure, origList :=
		saveConfigAsLast, createSessions, openITerm2Grid, configureAgentsAsync, listFleetSessions
	origEnsureDash, origConfigureDash := ensureDashboard, configureDashboard
	origProvPerms := provisionPermissions
	t.Cleanup(func() {
		saveConfigAsLast, createSessions, openITerm2Grid, configureAgentsAsync, listFleetSessions =
			origSave, origCreate, origGrid, origConfigure, origList
		ensureDashboard, configureDashboard = origEnsureDash, origConfigureDash
		provisionPermissions = origProvPerms
	})
	saveConfigAsLast = func(cfg *config.FleetConfig) error {
		s.savedURL = cfg.Project.RelayURL
		return nil
	}
	createSessions = func(cfg *config.FleetConfig, claudeBin string) []runner.LaunchResult {
		var results []runner.LaunchResult
		for _, a := range cfg.Agents {
			results = append(results, runner.LaunchResult{Agent: a.Name, Success: true, Action: "created"})
		}
		return results
	}
	openITerm2Grid = func(project string, agents []string) error { return nil }
	configureAgentsAsync = func(cfg *config.FleetConfig) (string, error) {
		s.configuredURL = cfg.Project.RelayURL
		return "", nil
	}
	listFleetSessions = func() ([]string, error) { return nil, nil }
	ensureDashboard = func() (string, error) { return "", nil }
	configureDashboard = func(string, string) (bool, error) { return false, nil }
	provisionPermissions = func(cwd string) error { s.permissionsCwd = cwd; return nil }
	return s
}
```

Add the new test:

```go
// launch must provision the scoped relay allowlist for the project cwd so woken
// agents run the relay lifecycle without a permission prompt.
func TestLaunchProvisionsRelayPermissions(t *testing.T) {
	seams := installLaunchSeams(t)
	var hits int
	relaySrv := fakeRelay(t, &hits)
	setFlagRelayURL(t, "")

	cfg := launchConfig(relaySrv.URL)
	cfg.Project.Cwd = t.TempDir()
	if err := launch(cfg, false); err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if seams.permissionsCwd != cfg.Project.Cwd {
		t.Errorf("launch must provision relay permissions for the project cwd %q, got %q", cfg.Project.Cwd, seams.permissionsCwd)
	}
}

// A permission-provisioning failure is a warning, not a launch abort — the
// fleet still comes up (the allowlist is a convenience, not a hard dependency).
func TestLaunchToleratesPermissionProvisionError(t *testing.T) {
	installLaunchSeams(t)
	var hits int
	relaySrv := fakeRelay(t, &hits)
	setFlagRelayURL(t, "")
	provisionPermissions = func(cwd string) error { return fmt.Errorf("disk full") }

	cfg := launchConfig(relaySrv.URL)
	cfg.Project.Cwd = t.TempDir()
	if err := launch(cfg, false); err != nil {
		t.Errorf("a permission-provision error must not fail the launch, got %v", err)
	}
}
```

(`installLaunchSeams` already stubs `provisionPermissions`; this test re-stubs it to fail after the helper runs. The `t.Cleanup` registered by the helper still restores the original.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/fleet/ -run TestLaunchProvisionsRelayPermissions -v`
Expected: compile error — `undefined: provisionPermissions`.

- [ ] **Step 3: Add the seam variable**

In `cmd/fleet/main.go`, in the `var (…)` block at lines 37-44, add `provisionPermissions` (alongside `createSessions` etc.):

```go
var (
	saveConfigAsLast     = config.SaveAsLast
	createSessions       = runner.CreateSessions
	openITerm2Grid       = runner.OpenITerm2Grid
	configureAgentsAsync = runner.ConfigureAgentsAsync
	provisionPermissions = runner.ProvisionRelayPermissions
	ensureDashboard      = dashboard.EnsureInstalled
	configureDashboard   = dashboard.Configure
)
```

- [ ] **Step 4: Call it in `launch()`**

In `cmd/fleet/main.go`, immediately after the existing `ProvisionMCP` block:

```go
	if err := runner.ProvisionMCP(cfg.Project.Cwd, relayURL); err != nil {
		fmt.Printf("  ⚠ Could not provision .mcp.json: %v\n", err)
	}
```

add:

```go
	if err := provisionPermissions(cfg.Project.Cwd); err != nil {
		fmt.Printf("  ⚠ Could not provision relay permissions: %v\n", err)
	}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./cmd/fleet/ -run TestLaunch -v`
Expected: PASS (all `TestLaunch*` tests, including the new one).

- [ ] **Step 6: Commit**

```bash
git add cmd/fleet/main.go cmd/fleet/launch_test.go
git commit -m "feat(launch): provision relay permission allowlist on every launch"
```

---

### Task 3: Wizard skip-all autonomy toggle (opt-in)

**Files:**
- Modify: `internal/wizard/wizard_model.go` (struct field; key handler; `Result()`; `View()`)
- Test: `internal/wizard/wizard_model_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/wizard/wizard_model_test.go`:

```go
// P on the agents panel toggles fleet-wide skip-all autonomy; off by default.
func TestWizardAutonomyToggle(t *testing.T) {
	m := newWizardModel(nil)
	m.activePanel = panelRight
	if m.skipPerms {
		t.Fatal("autonomy must be OFF by default")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	wm := updated.(wizardModel)
	if !wm.skipPerms {
		t.Fatal("P must toggle skip-all autonomy ON")
	}
	updated, _ = wm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	wm = updated.(wizardModel)
	if wm.skipPerms {
		t.Fatal("P must toggle skip-all autonomy back OFF")
	}
}

// Skip-all OFF → no claude flags; ON → --dangerously-skip-permissions, and it
// survives the TOML round-trip so --last relaunches with the same posture.
func TestWizardAutonomyFlagsInResult(t *testing.T) {
	build := func(skip bool) *config.FleetConfig {
		m := newWizardModel(nil)
		m.agents.SetAgents([]agentItem{
			{agent: config.AgentConfig{Name: "dev", Color: "green", Role: "Lead"}, enabled: true},
		})
		m.project.projName = "p"
		m.project.pathInput.SetValue("/tmp")
		m.skipPerms = skip
		m.launching = true
		res := m.Result()
		if res == nil {
			t.Fatal("expected a wizard result")
		}
		return res.Config
	}

	if flags := build(false).Claude.Flags; len(flags) != 0 {
		t.Errorf("autonomy OFF must leave claude flags empty, got %v", flags)
	}

	cfg := build(true)
	if len(cfg.Claude.Flags) != 1 || cfg.Claude.Flags[0] != "--dangerously-skip-permissions" {
		t.Fatalf("autonomy ON must set the skip-permissions flag, got %v", cfg.Claude.Flags)
	}

	path := filepath.Join(t.TempDir(), "autonomy.toml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Claude.Flags) != 1 || loaded.Claude.Flags[0] != "--dangerously-skip-permissions" {
		t.Errorf("skip-permissions flag must survive the TOML round-trip, got %v", loaded.Claude.Flags)
	}
}

// P must not toggle autonomy while a text input has focus — it's a plain
// character typed into the path/relay field, not a shortcut.
func TestWizardAutonomyNotToggledInTextInput(t *testing.T) {
	m := newWizardModel(nil)
	m.activePanel = panelLeft
	m.project.focus = focusPath
	m.project.pathInput.Focus()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	wm := updated.(wizardModel)
	if wm.skipPerms {
		t.Error("P typed into the path field must not toggle autonomy")
	}
}

// The autonomy posture is visible in the rendered view.
func TestWizardViewShowsAutonomy(t *testing.T) {
	m := newWizardModel(nil)
	m.activePanel = panelRight
	if !strings.Contains(m.View(), "Autonomy") {
		t.Errorf("View must show the autonomy posture; got:\n%s", m.View())
	}
	m.skipPerms = true
	if !strings.Contains(m.View(), "SKIP-ALL") {
		t.Errorf("View must flag SKIP-ALL when autonomy is on; got:\n%s", m.View())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/wizard/ -run TestWizardAutonomy -v`
Expected: compile error — `m.skipPerms undefined`.

- [ ] **Step 3: Add the `skipPerms` field**

In `internal/wizard/wizard_model.go`, in the `wizardModel` struct's State group (after `status string`), add:

```go
	skipPerms bool // fleet-wide --dangerously-skip-permissions, toggled with P
```

- [ ] **Step 4: Add the `P` key handler**

In `internal/wizard/wizard_model.go` `Update`, inside the `if !isTextInput {` switch, after the `case "s":` block (before the closing `}` of the switch), add:

```go
			case "P":
				// Toggle fleet-wide full autonomy. Agents panel only — it's the
				// finalize screen before launch; the flag applies to the whole fleet.
				if m.activePanel == panelRight {
					m.skipPerms = !m.skipPerms
					return m, nil
				}
```

- [ ] **Step 5: Set the flag in `Result()`**

In `internal/wizard/wizard_model.go` `Result()`, after the `cfg := &config.FleetConfig{…}` literal and before `return &WizardResult{…}`, add:

```go
	if m.skipPerms {
		cfg.Claude.Flags = []string{"--dangerously-skip-permissions"}
	}
```

- [ ] **Step 6: Render the autonomy indicator and update help**

In `internal/wizard/wizard_model.go` `View()`, change the right-panel help string (the final `else` branch) from:

```go
		help = "j/k=move  space=toggle  e=edit  n=new  d=del  a=all  enter=launch  s=save+launch  tab=presets  q=quit"
```

to:

```go
		help = "j/k=move  space=toggle  e=edit  n=new  d=del  a=all  P=autonomy  enter=launch  s=save+launch  tab=presets  q=quit"
```

Then, after the panel content is written (after `sb.WriteString(content)` and its following `sb.WriteString("\n")`) and BEFORE the `if m.status != ""` block, add the indicator:

```go
	// Autonomy posture — fleet-wide permission stance, toggled with P.
	if m.skipPerms {
		autonomyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		sb.WriteString(autonomyStyle.Render("  ⚠ Autonomy: SKIP-ALL — agents skip every permission prompt") + "\n")
	} else {
		sb.WriteString(dimStyle.Render("  Autonomy: prompts  (P: skip all permissions)") + "\n")
	}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/wizard/ -run TestWizard -v`
Expected: PASS (existing wizard tests + the 4 new `TestWizardAutonomy*`/`TestWizardViewShowsAutonomy`).

- [ ] **Step 8: Commit**

```bash
git add internal/wizard/wizard_model.go internal/wizard/wizard_model_test.go
git commit -m "feat(wizard): P toggles opt-in --dangerously-skip-permissions autonomy"
```

---

### Task 4: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Run the whole suite**

Run: `go test ./...`
Expected: every package `ok`.

- [ ] **Step 2: Build the binary**

Run: `go build -o /tmp/fleet-verify ./cmd/fleet && echo OK`
Expected: `OK`, no errors.

- [ ] **Step 3: Sanity-check the allowlist writer against a temp dir**

Run:
```bash
cat > /tmp/permcheck.go <<'EOF'
package main

import (
	"fmt"
	"os"
	"github.com/zairedegrees/fleet/internal/runner"
)

func main() {
	dir, _ := os.MkdirTemp("", "permcheck")
	if err := runner.ProvisionRelayPermissions(dir); err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	b, _ := os.ReadFile(dir + "/.claude/settings.local.json")
	fmt.Print(string(b))
}
EOF
go run /tmp/permcheck.go && rm -f /tmp/permcheck.go
```
Expected: JSON containing `"permissions": { "allow": [ "mcp__agent-relay__*" ] }`.

- [ ] **Step 4: Final commit (if any uncommitted verification fixups)**

```bash
git status --short
```
Expected: clean (all work already committed in Tasks 1-3).

---

## Notes for the implementer

- The launch path already turns `Config.Claude.Flags` into the `claude` command line in `runner.CreateSessions` (`internal/runner/runner.go:30-33`), so Task 3 needs no runner change — only the wizard must populate the field.
- `internal/runner/mcpjson.go` is the reference pattern for Task 1's merge/backup/refuse-malformed behavior — keep them structurally consistent.
- Do not change `dashboard.Configure`: it skips when a status line already exists, but the allowlist must always be written, so it lives as its own unconditional `launch()` step. Both write `settings.local.json` via independent non-destructive merges and run sequentially, so there is no clobber.
- `settings.local.json` is already covered by the dashboard's `.claude/.gitignore` management — no extra gitignore work needed.
