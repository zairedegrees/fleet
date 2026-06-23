# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/), and this project adheres to
[Semantic Versioning](https://semver.org/).

## [0.3.0] — 2026-06-23

### Added
- **`bounded` agent posture + supervisor.** Agent posture graduates from the
  `auto_talk` boolean to a three-tier enum: `idle` (default, zero tokens, woken
  on dispatch), `bounded` (proactive re-wake under a hard daily cap), and
  `always` (greets at boot, the old `auto_talk = true`). A `bounded` agent is
  re-woken on its own cadence by an auto-spawned, per-project **supervisor**
  (`fleet supervise`) that enforces `interval`, `active_hours`,
  `max_wakes_per_day`, and a daily `budget_usd`, with automatic backoff when an
  agent keeps finding no work. The supervisor starts detached at launch when any
  agent is bounded and is stopped by `fleet --kill`; it fails safe (if it dies,
  agents fall back to idle). Configure via `posture = "bounded"`, optional
  per-agent `[agents.bounded]`, and fleet-wide `[bounded_defaults]`.
- **Posture + budget in `fleet --status`** (per-agent `bounded` label with live
  `wakes N/max · ~$spent/$budget`, plus a supervisor running/stopped line) and a
  projected daily **cost estimate** for bounded agents in `fleet usage`. Cost is
  always a labelled estimate, never a measured figure.

### Changed
- `auto_talk` is now a back-compatible alias: `auto_talk = true` loads as
  `posture = "always"`; existing configs keep working unchanged.

## [0.2.0] — 2026-06-19

### Added
- **Prebuilt binaries** for macOS and Linux (`amd64` + `arm64`), published on
  every tagged release by goreleaser, with SHA-256 checksums.
- **Install script** (macOS & Linux): `curl -fsSL …/scripts/install.sh | sh` —
  detects OS/arch, verifies the checksum, and installs to your bin dir.
- **`fleet --demo`.** A zero-prerequisite, animated walkthrough of the live
  status view against a scripted in-memory fleet — no tmux, relay, or Claude
  Code required. Agents flip idle↔working so you can see the UI breathe; ctrl+c
  exits, nothing is touched.

### Changed
- `fleet --version` is now stamped from the release tag at build time.

## [0.1.10] — 2026-06-18

### Added
- **`fleet --status --watch`.** Live-refreshing status: clears and reprints the
  per-agent view every `--interval` (default 2s) until ctrl+c, so you can watch
  agents flip idle↔working in real time. Read-only (relay + tmux) — an idle
  fleet still costs zero tokens; it just repeats the existing `--status` reads.
  On a slow relay the previous frame stays until the new one is ready (no blank
  flicker). One-shot `fleet --status` is unchanged.

## [0.1.9] — 2026-06-18

### Added
- **Wake-on-mission.** When a task is dispatched to an agent — by any agent, not
  just the operator's `fleet dispatch` CLI — the recipient's pane is now woken
  automatically if it is dormant. The built-in coordination core emits a
  post-commit event on `dispatch_task` and runs a lightweight reconciliation
  sweep; a new `register_notify_channel` tool (operator-only) records each
  agent's tmux session at launch. Watching is entirely server-side (tmux + SQL),
  so an idle agent with no work still costs zero tokens, and a busy agent is
  never interrupted — it picks the task up in its running talk loop.

## [0.1.8] — 2026-06-17

### Changed
- **Wizard daily-UX.** The interactive wizard now lists saved projects
  most-recently-used first (by config mtime) and pre-selects the last-launched
  one, turns the left panel into a navigable settings hub (Path / Relay / Team)
  so every setting stays reachable in-flow, and uses a consistent `esc` ladder
  that walks up one level and only quits from the project list. No new config,
  no new knobs — recency derives from state already maintained on every launch.
- **Wizard-first `/fleet` onboarding skill.** The bundled skill now leads with
  the interactive wizard (which writes and validates the config and symlinks
  `last.toml` itself) instead of hand-authoring TOML, with a quick start up top
  and a compact headless-setup fallback for non-interactive use. Documentation
  only.

## [0.1.7] — 2026-06-17

### Changed
- **Broadcast is executive-only.** `send_message(to="*")` is now restricted to
  agents flagged `is_executive`; a non-executive (or anonymous/unregistered)
  sender is rejected with a clear error and the message is not created. Direct
  messages are unchanged — the gate applies only to `*`. This enforces the
  permission the system already advertised to executives.

## [0.1.6] — 2026-06-17

### Added
- **Goals.** High-level objectives that group tasks — three new coordination-core
  tools: `create_goal` (open a named goal), `get_goal` (metadata + derived
  progress: total / done / in_progress / blocked), and `list_goals` (compact,
  each with done/total). A new `goals` table is auto-migrated. The `/relay` skill
  teaches the workflow.

### Changed
- **`dispatch_task` can attach a task to a goal.** A new optional `goal_id`
  routes the task under a goal; an unknown id is rejected (start the goal first).
  Without it, behavior is unchanged. Goal progress is derived from the goal's
  non-archived tasks (cancelled excluded), never stored.
- **`list_tasks` gains a `goal_id` filter** — fetch a goal's tasks via the
  existing tool instead of duplicating them in `get_goal`.

## [0.1.5] — 2026-06-17

### Added
- **Agent-to-agent conversations.** Three new coordination-core tools —
  `start_conversation` (open a named thread, optionally posting the opening
  message in one call), `get_conversation` (paginated, truncated thread fetch),
  and `list_conversations` (your threads with message and unread counts). A new
  `conversations` table is auto-migrated. The `/relay` skill teaches agents the
  workflow; thread messages still arrive in the normal inbox, so an agent pulls
  the full thread only when it needs the broader context.

### Changed
- **Leaner agent tool catalog.** `tools/list` now advertises only agent-facing
  tools. The operator-only tools (`register_agent`, `register_profile`,
  `deactivate_agent`, `list_orgs`) are still served on `tools/call` — the fleet
  CLI calls them by name — but are dropped from every agent's catalog, trimming
  ~780 context tokens per agent. Dropping `register_agent` also enforces the
  no-self-register design (an agent can't call a tool it never sees).
- **`send_message` enforces conversation integrity.** A `conversation_id` that
  doesn't exist is rejected (start the conversation first); a valid one bumps
  the thread's last-activity timestamp in the same transaction.

### Docs
- The README's token-economy claim is now backed by a real, transcript-based
  measurement (an idle "check inbox" turn costs tens of thousands of tokens, not
  ~1k) instead of the previous, underivable "95%".

## [0.1.4] — 2026-06-15

### Added
- **Readable fleet status.** `fleet --status` now derives each agent's operator
  state — `idle` (registered, no active task), `working` (active task), or
  `registered` (task count unknown) — instead of the misleading raw
  `relay: active · 0 task(s)`. Each line also carries its config posture
  (`auto-talk` vs `on-demand`) and a `seen Xm ago` stamp parsed from the relay's
  `last_seen` (already on the wire), uses the short agent name (the project is
  the group header), and a one-line legend explains the `idle` standby state and
  how to wake an agent — shown only when an agent is actually idle.
- **Launch recap explains the quiet-by-design posture.** After a launch the recap
  states that agents register, take their role, then go quiet (token discipline),
  counts how many greet at boot (`auto-talk`) vs wait `on-demand`, and points to
  `fleet dispatch` and `fleet --status` — instead of an unexplained "watch the
  panes".

No coord wire-contract change; agent behavior is unchanged (token discipline
preserved) — this release is pure legibility.

## [0.1.3] — 2026-06-14

### Added
- **Per-agent behavioral config.** `[[agents]]` entries gain `model`, `persona`,
  `skills`, `tools`, and `permission_mode` — all optional and `omitempty`, so
  existing `~/.fleet` configs load and re-save byte-identically.
- **Per-agent launch.** Agents now launch with `--model`, `--permission-mode` and
  `--allowedTools`, and their persona is injected via `--append-system-prompt-file`
  (written to `~/.fleet/personas/<project>-<agent>.txt`). The multiline persona
  never rides a `tmux send-keys` line — only its file path does — so quotes,
  `$`, backticks and newlines are inert. Tool allow-lists always re-include
  `mcp__agent-relay__*` so a narrowed scope never breaks task routing.
- **Personas ready for any project, out of the box.** At launch, an agent whose
  name matches a known role (`dev`, `auditor`, `ops`, `frontend`, `ux-designer`,
  `researcher`, `quant`, `architect`, `security`, `docs`, `notifier`) fills its
  empty behavioral fields from that role's profile — so existing fleets get
  ready-made personas with no config change. Explicit config values always win;
  an agent named after no known role launches bare (byte-identical to v0.1.2).
  The config on disk is never rewritten — defaults are resolved per launch.
- **Behavioral preset library v2.** A canonical identity per role (model tier,
  persona, skills, tool scope, permission posture) backs all presets. The 7
  existing presets are tuned in place and three new ones ship — Solo Pair ⚡⚡
  (cheap hands, Opus eyes), Design Studio 🎨, Security Hardening 🛡. Personas
  carry an idle-discipline clause and preset agents stay `auto_talk = false`.
- **Wizard agent drawer** now edits Model and Permission (selects) and a
  multiline Persona (textarea: Enter inserts a newline, Tab/Ctrl+S leaves). When
  fleet-wide skip-all is on, the Permission row says the per-agent posture is
  ignored rather than showing one it won't honor.

### Changed
- The agent drawer is table-driven (`[]fieldSpec`): adding a field is one row,
  not three hardcoded chains.

## [0.1.2] — 2026-06-14

### Fixed
- The agent-configure choreography now settles before its first command, so the
  `/rename` sent right after a pane's prompt appears is no longer dropped while
  Claude Code is still loading MCP servers — agents reliably get their names.
  (Pre-existing; surfaced on a live launch.)

## [0.1.1] — 2026-06-14

### Added
- **Native coordination core (`internal/coord`)** — a self-contained, MIT,
  in-binary MCP-over-HTTP server backed by pure-Go SQLite (`modernc`, no CGO). It
  is an independent reimplementation of the wrai.th relay's wire contract (same
  endpoint, same 19 tools) and is now the **default backend**: a fresh launch
  starts it on `localhost:8090` with no download, no AGPL, and no consent prompt.
- `internal/coordmgr` runs `coord` as a detached `fleet coord serve` child that
  outlives the launch, with its own pidfile/flock lifecycle and graceful SIGTERM
  shutdown.
- Full MCP handshake (`initialize` + `tools/list`) so a real Claude Code client
  discovers and calls coord's tools; verified end-to-end against `claude -p`.
- An original MIT `/relay` skill (`skill/relay/SKILL.md`), embedded and installed
  for the embedded backend — no network fetch.
- `fleet --version`.

### Changed
- The coordination backend is selectable: `--relay-backend embedded|download`,
  `FLEET_RELAY_BACKEND`, or the project's `relay_backend`. The downloaded AGPL
  `agent-relay` binary is now an explicit opt-in fallback.
- `fleet relay start|stop|status` and `fleet --doctor` are backend-aware, and
  user-facing output drops the stale "wrai.th relay" label (now "coordination
  core").
- README and the `/fleet` onboarding skill updated for the built-in core.

### Fixed
- Install the `/relay` skill on every embedded launch, even when coord is already
  running — it sat behind the reachability short-circuit, so a woken agent could
  be left with no skill to resolve `/relay talk`.

## [0.1.0] — 2026-06-11

### Added
- Initial public release: Bubble Tea wizard, tmux sessions + iTerm2 grid, task
  dispatch + wake, `fleet add`/`stop`/`usage`/`logs`, relay-backed `--status`,
  and a doctor with install hints.
- Server-side, non-destructive agent registration (agents never self-register),
  ANSI sanitization of relay-sourced strings, and a bundled per-agent dashboard.
- A managed relay backend (downloaded agent-relay binary) and the `/fleet`
  onboarding skill.

[0.1.2]: https://github.com/zairedegrees/fleet/releases/tag/v0.1.2
[0.1.1]: https://github.com/zairedegrees/fleet/releases/tag/v0.1.1
[0.1.0]: https://github.com/zairedegrees/fleet/releases/tag/v0.1.0
