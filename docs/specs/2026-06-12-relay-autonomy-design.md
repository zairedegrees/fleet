# Relay autonomy for launched agents

**Date:** 2026-06-12
**Branch:** `fix/agent-autonomy-permissions`

## Problem

Agents launched through the wizard run plain `claude` — the wizard-produced
config has an empty `[claude]` section, so `ClaudeConfig.Flags` is never set.
Without `--dangerously-skip-permissions`, every MCP tool call triggers Claude
Code's interactive "Do you want to proceed?" prompt.

Observed in an end-to-end run: a woken agent stalled four times in a row
(`get_session_context`, `claim_task`, `start_task`, `complete_task`), each
needing a manual approval in its pane. This defeats fleet's core promise —
"wake an agent on dispatch, it works autonomously." The existing
`polymarket-bot-brain` config only works unattended because its
`flags = ["--dangerously-skip-permissions"]` was added by hand, not by the
wizard.

## Goals

- A freshly launched fleet runs the relay `/relay talk` lifecycle unattended,
  out of the box, with no manual approvals.
- The safe default does **not** grant blanket permission to run arbitrary
  shell commands or edit files — only the relay's own MCP tools.
- Full autonomy (skip every prompt) stays available, but as a deliberate
  opt-in, not a silent default — appropriate for a public-facing tool.

## Decisions

- **Default = scoped relay allowlist.** Always provisioned at launch.
- **`--dangerously-skip-permissions` = opt-in toggle** in the wizard, OFF by
  default.
- **Allowlist scope = whole relay server** (`mcp__agent-relay__*`), not a
  curated subset. The relay is fleet's own trusted coordination channel;
  enumerating tools risks a stall the day `/relay talk` calls one we forgot.

## Fix 2 — Relay permission allowlist (default, unconditional)

New function `runner.ProvisionRelayPermissions(cwd string) error`, called from
`launch()` immediately after `runner.ProvisionMCP`, mirroring its structure.

It merges into `<cwd>/.claude/settings.local.json`:

```json
{ "permissions": { "allow": ["mcp__agent-relay__*"] } }
```

Rule format confirmed against Claude Code 2.1.x docs: `mcp__<server>__*` is a
supported server-wide **allow** glob (the server segment must be glob-free).
Settings precedence places `settings.local.json` above shared
`settings.json`; permission rules are read at session startup, so the file
must be written before the agent's `claude` process boots — it already is,
since `launch()` provisions before `createSessions` spawns the panes.

Merge semantics — identical robustness to `ProvisionMCP` / `mergeStatusLine`:

1. Read existing file. Absent → start from `{}`. Malformed JSON → return an
   error, leave the file untouched (never clobber).
2. Back up the existing bytes to `settings.local.json.bak`.
3. Read `permissions.allow` (array). Append `mcp__agent-relay__*` only if not
   already present (dedup). Preserve every other key and array entry.
4. `MkdirAll` the `.claude` dir, write indented JSON + trailing newline.

`settings.local.json` is already covered by the `.claude/.gitignore` that the
dashboard manages, so nothing lands in the user's committed tree.

**Coexistence with the dashboard.** `dashboard.Configure` also writes
`settings.local.json` (the `statusLine` key) but skips when a status line is
already defined. The permission allowlist must be written regardless, so it is
its own unconditional step rather than folded into `Configure`. Both are
non-destructive merges and run sequentially in `launch()`, so there is no race
and no clobber. Each keeps its own `.bak` (last writer's backup wins, which is
acceptable — `.bak` is a convenience, not a transaction log).

**Launch wiring.** Add a seam variable in `cmd/fleet/main.go`
(`provisionPermissions = runner.ProvisionRelayPermissions`) alongside the
existing `createSessions` / `openITerm2Grid` seams, and call it next to
`ProvisionMCP`. A failure is surfaced as a warning (`⚠`) and does not abort the
launch — consistent with how `ProvisionMCP` failures are handled.

## Fix 1 — Skip-all autonomy toggle (opt-in)

`ClaudeConfig.Flags` already exists and is honored: `createSessions` appends
each flag to the `claude` command line. The only gap is that the wizard never
sets it.

- Add `skipPerms bool` to `wizardModel`.
- Bind **`P`** (agents panel only, not a text-input context) to toggle it.
- `Result()` sets `Config.Claude.Flags = []string{"--dangerously-skip-permissions"}`
  when `skipPerms` is true, and leaves it `nil` when false.
- The flag persists into the saved TOML and is carried by `fleet --last`.

### Wizard UI

In the right (agents) panel — the finalize screen before `enter` = launch:

- A status line by the panel header: `Autonomy: prompts` (default) or
  `Autonomy: SKIP-ALL ⚠` (toggled on).
- Help bar gains `P=autonomy`:
  `j/k=move  space=toggle  e=edit  n=new  d=del  a=all  P=autonomy  enter=launch  s=save+launch  tab=presets  q=quit`

When skip-all is ON, the relay allowlist (Fix 2) is still written but is moot —
the flag overrides all permission rules. That is fine and intended: the
allowlist is the always-on floor, the flag is an optional override on top.

## Testing (TDD)

- `runner.ProvisionRelayPermissions` (new `internal/runner/permissions_test.go`,
  mirroring `mcpjson_test.go`):
  - fresh file created with the allow entry;
  - existing unrelated keys preserved;
  - existing `permissions.allow` entries preserved, new entry appended;
  - idempotent — running twice does not duplicate the entry;
  - malformed existing JSON → error, file untouched, no `.bak` overwrite of good data;
  - `.bak` written when an existing file is present.
- Wizard (`internal/wizard/wizard_model_test.go`):
  - `P` flips `skipPerms`;
  - `Result()` sets `Claude.Flags` to `["--dangerously-skip-permissions"]` when on, `nil` when off;
  - `P` is inert while a text input (path / relay URL / drawer) has focus;
  - help bar / autonomy indicator reflects state.
- `cmd/fleet` launch pipeline (`launch_test.go`):
  - `provisionPermissions` seam is invoked during `launch()`;
  - a seam error is reported as a warning and does not fail the launch.

## Out of scope

- Curated per-tool relay allowlists (whole-server chosen instead).
- Live reload of permission rules (Claude Code reads them at startup only).
- Changing the existing `polymarket-bot-brain` config.
