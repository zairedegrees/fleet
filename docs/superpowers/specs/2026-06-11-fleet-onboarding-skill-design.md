# Fleet Onboarding Skill — Design

**Date:** 2026-06-11
**Status:** Approved, ready for implementation plan

## Goal

Ship a Claude Code skill, bundled in the fleet repo, that takes a brand-new user
from "I cloned fleet" to "my first fleet is running and I understand how it
works" — with Claude driving the setup. The skill exists because fleet's value
(multi-agent orchestration) has real first-run friction: prerequisites, a
required relay, a config to author, and a mental model (idle agents, dispatch to
wake, profile-routed tasks) that isn't obvious. The skill collapses that into a
guided, Claude-driven 0→1.

## Decisions (validated during brainstorming)

1. **Scope:** 0→1 setup only — first fleet running. NOT a day-2 operations
   reference (status/logs/kill are touched only for verification).
2. **Pilotage:** Claude drives. It authors the config rather than sending the
   user to the interactive TUI wizard (which Claude cannot drive).
3. **Team composition:** Claude reads the target repo and proposes a tailored
   roster (agents, roles, hierarchy, auto_talk), with reasoning, for the user to
   validate/adjust — leaning on Claude's strength over the wizard's heuristic
   scan.
4. **Distribution:** the skill lives in the repo as markdown; the README gives a
   one-liner to install it into Claude Code. Zero fleet code change.
5. **Consent:** the skill never installs a system service (the relay) without
   asking first. It detects, explains, and proposes — the user approves before
   anything touches their system.

## The flow Claude runs

Triggered when the user asks to set up / create their first fleet (or invokes
`/fleet`). Claude executes, narrating and pausing for input where it matters:

1. **Preflight.** Run `fleet --doctor`. If the `fleet` binary isn't on PATH,
   offer to `go build -o fleet ./cmd/fleet`. Resolve missing prerequisites with
   the doctor's install hints (tmux, Claude Code CLI). Treat the **relay** as
   the critical dependency: check whether one is reachable (default
   `http://localhost:8090/mcp`); if not, explain it's required and **ask** before
   installing — propose the wrai.th one-liner + `agent-relay serve`. Never
   auto-install.
2. **Understand the project.** Read the target repo (the cwd the fleet will work
   in): stack, structure, README, to understand the work the fleet will do.
3. **Propose the team.** Present a tailored roster: N agents with names, roles,
   hierarchy (who is `is_executive`, who `reports_to` whom), and which should
   `auto_talk`, each with a one-line rationale. The user validates or adjusts.
4. **Author the config.** Write the TOML — `[project]` (name, cwd, relay_url),
   `[claude]` flags, and `[[agents]]` — to `~/.fleet/last.toml` and
   `~/.fleet/configs/<name>.toml`. Respect fleet's validation: agent/project
   names match `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, roles avoid the blocklist
   (`" ' ` $ \` plus newlines/tabs), colors are from fleet's palette.
5. **Launch.** Run `fleet --last`.
6. **Verify and explain the model.** Run `fleet --status` to confirm
   registration, then explain the mental model in two or three sentences: agents
   boot **idle** (zero tokens), you **dispatch** a task to wake one, the relay
   routes tasks by `profile_slug`. Point at `fleet usage` and `fleet logs` for
   later.
7. **First dispatch (optional).** Offer to dispatch a starter task to one agent
   (`fleet dispatch "<task>" --to <agent>`) so the user sees the loop work end
   to end.

## Launch mechanism

No new fleet command. `fleet --last` loads `~/.fleet/last.toml` (see
`internal/config.LoadLast`). Claude writes the authored config there (and a copy
to `~/.fleet/configs/<name>.toml` for persistence and `fleet usage`/`--status`
resolution), then runs `fleet --last`. This reuses the existing, tested launch
path with zero code change.

*Residual (out of scope for v1):* a `fleet launch <name>` flag would make this
more robust than hand-writing `last.toml`, but it is not required — the skill
works entirely within today's CLI.

## File structure & distribution

- **Repo:** `skill/fleet/SKILL.md` — frontmatter (`name: fleet`, a `description`
  whose trigger phrasing covers "set up fleet", "create/launch my first fleet",
  "onboard me to fleet", `/fleet`) followed by the flow above. Prescriptive on
  the seven steps; leaves team composition to Claude's judgment. Modeled on
  wrai.th's `skill/relay.md` (concise, sectioned, imperative).
- **Install:** a README one-liner symlinking the skill into Claude Code, e.g.
  `ln -s "$(pwd)/skill/fleet" ~/.claude/skills/fleet`. Documented in a short
  "Onboarding skill" README subsection.

## Scope boundaries

**In scope:** the seven-step 0→1 flow; Claude-driven config authoring; tailored
team proposal; consent before relay install; the install one-liner + README note.

**Out of scope (YAGNI):**
- A day-2 command reference (the skill is onboarding, not a manual).
- A fleet installer for the binary itself (preflight offers `go build`; it does
  not package or distribute fleet).
- A `fleet launch <name>` CLI addition (noted as a possible later robustness win).
- `fleet skill install` tooling or a Claude Code plugin package (manual symlink
  install was the chosen distribution).

## Success criteria

- A user who has cloned fleet and has Claude Code can say "set up fleet for this
  project" and reach a running, registered fleet — prerequisites resolved, relay
  running (with their consent), a tailored config authored, `fleet --status`
  green — without ever opening the TUI wizard.
- The skill never installs a system service without explicit approval.
- No change to fleet's Go code; the skill is markdown plus a README note.

## Validation approach

Because the deliverable is a skill (markdown) plus docs, validation is by dry-run
walkthrough rather than Go tests:
- Self-review the SKILL.md against the seven steps for correct command syntax
  (`fleet --doctor`, `--status`, `--last`, `dispatch --to`), correct config TOML
  shape, and the validation rules.
- Confirm the trigger description actually fires on the intended phrasings.
- A manual end-to-end dry run on a throwaway project (optional, like the relay
  e2e) to confirm the authored-config → `fleet --last` path launches cleanly.
