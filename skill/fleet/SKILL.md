---
name: fleet
description: Set up and launch a multi-agent Claude Code fleet for a project — first-run onboarding. Use when the user wants to set up fleet, create or launch their first fleet, onboard to fleet, or types /fleet. Claude checks prerequisites, reads the repo to recommend a tailored team, then guides launch through the interactive wizard (which writes and validates the config itself). A compact headless path is included for non-interactive setup.
---

# Fleet Onboarding — 0 → first running fleet

Drive the user from a cloned repo to a running fleet. You advise and narrate; the visual wizard is driven by the human at the keyboard. Pause for input where marked. **Never install a system service without asking first.**

## Quick start

1. `which fleet` — if missing, `go install github.com/zairedegrees/fleet/cmd/fleet@latest` (or, in the repo, `go build -o fleet ./cmd/fleet`).
2. `fleet --doctor` — checks tmux, the Claude Code CLI, and the built-in coordination core.
3. `fleet` — the wizard. You recommend a team (step 2 below); the user picks it and launches.

## 1. Prerequisites

Run `which fleet`. If absent, install with `go install github.com/zairedegrees/fleet/cmd/fleet@latest`, or build from the repo: `go build -o fleet ./cmd/fleet` (then run `./fleet` or move it onto PATH).

Run `fleet --doctor`: it checks tmux, the Claude Code CLI, iTerm2 (optional grid layout), and the coordination core, each with an install hint. Resolve missing tools with the hints shown.

Coordination is **built into the binary** — nothing to install. On launch fleet starts its native core (`coord`) on `localhost:8090` and installs the `/relay` skill automatically; no download, no consent. (Point elsewhere with `fleet --relay-url <url>`; the older AGPL relay is opt-in via `fleet --relay-backend download`.)

## 2. Understand the project and recommend a team

Read the repo the fleet will work in (stack, structure, README, entry points) so you know what work the agents will do.

Propose a roster as a short table — for each agent: name, role, reports-to, and whether it's the executive. Pick 3–6 agents that fit the project, one line each. Ask the user to confirm or adjust.

Rules:
- Exactly one `is_executive` agent — the project owner / decision-maker.
- Every worker `reports_to` the executive.
- `auto_talk = false` by default (idle = zero tokens); set `auto_talk = true` only for an agent that must poll continuously.

## 3. Launch via the wizard

Have the user run `fleet`. The wizard lists saved projects (most-recent first, last-launched pre-selected); they pick or add the project, confirm its path and relay URL (validated on the spot, defaults to the local relay), choose the team you recommended (the closest preset, adjustable), and launch.

The wizard **writes and validates the config and points `last.toml` at it automatically** — you don't author any TOML on this path. It's a TUI driven by the human; you guide and narrate.

## 4. Verify and explain the model

Run `fleet --status` and confirm each agent is registered and **idle**. Then explain, in two or three sentences:
- Agents boot **idle** — zero tokens until they have work.
- **Dispatch** wakes an agent: `fleet dispatch "<task>" --to <agent>`.
- `fleet usage` shows polling-vs-idle and task counts; `fleet logs <agent> -f` streams a pane.

## 5. First dispatch (optional)

Offer a starter task so the user sees the loop end to end: `fleet dispatch "<small real task>" --to <agent>`, then `fleet logs <agent> -f` to watch it pick up.

## Headless setup (no TUI)

When you should set fleet up without the wizard (a remote/headless session, or the user asks you to do it all): write the config yourself, then relaunch it.

Write `~/.fleet/configs/<project>.toml`, then symlink it as the last config:

```bash
ln -sf ~/.fleet/configs/<project>.toml ~/.fleet/last.toml
```

Use `ln -sf` — do NOT write into `~/.fleet/last.toml` directly: it's a symlink, and you'd overwrite another project's config. Then run `fleet --last`.

Minimal config:

```toml
[project]
  name = "acme-api"
  relay_url = "http://localhost:8090/mcp"
  cwd = "/absolute/path/to/project"

[[agents]]
  name = "dev"
  color = "green"
  role = "lead engineer"
  is_executive = true

[[agents]]
  name = "auditor"
  color = "blue"
  role = "reviews diffs"
  reports_to = "dev"
```

Rules `fleet` enforces (get them right so launch doesn't reject):
- `name` (project and agents): `^[a-zA-Z0-9][a-zA-Z0-9_-]*$` — alphanumerics, hyphens, underscores; no spaces or dots. Prefer lowercase.
- `role`: no `"`, `'`, backtick, `$`, `\`, or newlines/tabs.
- `color`: one of green, orange, blue, red, purple, pink, cyan, yellow.
- Add a `[claude]` table with `flags = ["--dangerously-skip-permissions"]` only for full autonomy.
