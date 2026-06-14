# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/), and this project adheres to
[Semantic Versioning](https://semver.org/).

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
- `fleet relay start|stop|status` and `fleet --doctor` are backend-aware.
- README and the `/fleet` onboarding skill updated for the built-in core.

## [0.1.0] — 2026-06-11

### Added
- Initial public release: Bubble Tea wizard, tmux sessions + iTerm2 grid, task
  dispatch + wake, `fleet add`/`stop`/`usage`/`logs`, relay-backed `--status`,
  and a doctor with install hints.
- Server-side, non-destructive agent registration (agents never self-register),
  ANSI sanitization of relay-sourced strings, and a bundled per-agent dashboard.
- A managed relay backend (downloaded agent-relay binary) and the `/fleet`
  onboarding skill.

[0.1.1]: https://github.com/zairedegrees/fleet/releases/tag/v0.1.1
[0.1.0]: https://github.com/zairedegrees/fleet/releases/tag/v0.1.0
