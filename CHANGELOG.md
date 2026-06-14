# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/), and this project adheres to
[Semantic Versioning](https://semver.org/).

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
  `mcp__agent-relay__*` so a narrowed scope never breaks task routing. A
  zero-behavioral agent launches byte-identically to v0.1.2.
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
