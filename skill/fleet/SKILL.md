---
name: fleet
description: Set up and launch a multi-agent Claude Code fleet for a project — first-run onboarding. Use when the user wants to set up fleet, create or launch their first fleet, onboard to fleet, or types /fleet. Claude drives: checks prerequisites, sets up the relay (with consent), proposes a tailored team, authors the config, and launches.
---

# Fleet Onboarding — 0 → first running fleet

Drive the user from a cloned repo to a running fleet. Narrate each step; pause
for input where marked. **Never install a system service without asking first.**

## 0. Is the fleet binary available?

Run `which fleet`. If it is missing, from the fleet repo build it:
`go build -o fleet ./cmd/fleet`, then run it as `./fleet` or move it onto PATH.

## 1. Preflight: `fleet --doctor`

Run `fleet --doctor`. It checks tmux, the Claude Code CLI, and iTerm2 (optional),
each with an install hint, plus the relay (which fleet manages — see below).
Resolve missing tools with the hints shown.

The relay is handled by fleet itself — no separate install. On the first launch,
if no relay is running, fleet asks for consent, then downloads the agent-relay
binary + the `/relay` skill and starts it locally. You do not run `agent-relay
serve` by hand. (Point at an existing/shared relay with `fleet --relay-url <url>`.)

## 2. Understand the project

Read the repo the fleet will work in (its cwd): stack, structure, README, entry
points. You are figuring out what work the agents will do.

## 3. Propose a tailored team

Propose a roster as a short table. For each agent give: name, role, reports-to,
and whether it is the executive. Pick 3–6 agents that fit the project and explain
each in one line. Ask the user to confirm or adjust.

Rules:
- Exactly one `is_executive` agent — the project owner / decision-maker.
- Every worker `reports_to` the executive.
- Default `auto_talk = false` for everyone (idle = zero tokens). Set
  `auto_talk = true` only for an agent that must poll continuously.

## 4. Author the config

Write the TOML to `~/.fleet/configs/<project>.toml`, then point `last.toml` at it
so `fleet --last` picks it up:

```bash
ln -sf ~/.fleet/configs/<project>.toml ~/.fleet/last.toml
```

Use `ln -sf` (fleet keeps `last.toml` as a symlink) — do NOT write the config
straight into `~/.fleet/last.toml`, because if it is already a symlink to another
project you would overwrite that project's config.

The config:

```toml
[project]
  name = "<project-slug>"
  relay_url = "http://localhost:8090/mcp"
  cwd = "<absolute-path-to-project>"

[claude]
  flags = ["--dangerously-skip-permissions"]

[[agents]]
  name = "<executive-name>"
  color = "green"
  role = "<one-line role>"
  is_executive = true

[[agents]]
  name = "<worker-name>"
  color = "blue"
  role = "<one-line role>"
  reports_to = "<executive-name>"
```

Validation (fleet rejects otherwise):
- `name` (project and agents) must match `^[a-zA-Z0-9][a-zA-Z0-9_-]*$` —
  alphanumerics, hyphens, underscores; no spaces or dots. Prefer lowercase (the
  relay lowercases agent names).
- `role` must not contain `"`, `'`, a backtick, `$`, `\`, or newlines/tabs.
- `color` must be one of: green, orange, blue, red, purple, pink, cyan, yellow.

## 5. Launch

Run `fleet --last`. It relaunches the config you just wrote: one tmux session per
agent, each registered on the relay, all idle.

## 6. Verify and explain the model

Run `fleet --status` and confirm each agent shows as registered. Then explain, in
two or three sentences:
- Agents boot **idle** — zero tokens until they have work.
- You **dispatch** a task to wake an agent:
  `fleet dispatch "<task>" --to <agent>`.
- `fleet usage` shows polling-vs-idle and task counts; `fleet logs <agent> -f`
  streams a pane.

## 7. First dispatch (optional)

Offer to send a starter task so the user sees the loop end to end:
`fleet dispatch "<small real task>" --to <agent>`, then `fleet logs <agent> -f`
to watch it pick up.

## Alternative: the visual wizard

A user who prefers a TUI can run bare `fleet` for the interactive wizard
(project → path → relay URL → team preset → launch) instead of steps 3–5.
