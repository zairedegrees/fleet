# Fleet Onboarding Skill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a bundled Claude Code skill that drives a new user from a cloned fleet repo to a running, registered first fleet.

**Architecture:** A single markdown skill file (`skill/fleet/SKILL.md`) instructing Claude through a seven-step, consent-gated 0→1 flow — doctor preflight, relay setup, repo scan + tailored team proposal, config authoring to `~/.fleet/last.toml`, launch via `fleet --last`, verify + explain. Plus a README "Onboarding skill" note with a symlink install one-liner. No Go code changes.

**Tech Stack:** Markdown (Claude Code skill format with YAML frontmatter); the existing fleet CLI (`--doctor`, `--last`, `--status`, `dispatch --to`, `logs`) and TOML config format.

**Validation note:** the deliverable is markdown, not Go — there are no unit tests. Validation is a correctness review of the skill's command syntax and config shape against the real CLI/validation rules (Task 3), plus an optional manual dry run. Spec: `docs/superpowers/specs/2026-06-11-fleet-onboarding-skill-design.md`.

---

### Task 1: Create the skill file

**Files:**
- Create: `skill/fleet/SKILL.md`

- [ ] **Step 1: Create `skill/fleet/SKILL.md` with this exact content**

````markdown
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

Run `fleet --doctor`. It checks tmux, the Claude Code CLI, iTerm2 (optional), and
the relay, each with an install hint. Resolve missing tools with the hints shown.

The **relay is required** (default `http://localhost:8090/mcp`). If doctor reports
it unreachable:
- Explain that fleet needs a running wrai.th relay to coordinate agents.
- **Ask before installing.** Propose:
  `curl -fsSL https://raw.githubusercontent.com/Synergix-lab/WRAI.TH/main/install.sh | bash`
  then `agent-relay serve`. Run it only after the user agrees. Never auto-install.

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

Write the same TOML to BOTH `~/.fleet/last.toml` and
`~/.fleet/configs/<project>.toml`:

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
- `name` (project and agents) must match `^[a-zA-Z0-9][a-zA-Z0-9_-]*$` — lowercase
  alphanumerics, hyphens, underscores; no spaces or dots.
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
````

- [ ] **Step 2: Verify the file was created and frontmatter is well-formed**

Run: `head -5 skill/fleet/SKILL.md`
Expected: the YAML frontmatter block with `name: fleet` and the `description:` line.

- [ ] **Step 3: Commit**

```bash
git add skill/fleet/SKILL.md
git commit -m "feat: add fleet onboarding skill"
```

---

### Task 2: Add the README install note

**Files:**
- Modify: `README.md` (add an "Onboarding skill" subsection under `## Install`)

- [ ] **Step 1: Read the current Install section to find the insertion point**

Run: `grep -n "^## " README.md`
Expected: locate `## Install` and the next heading (`## Requirements`). Insert the new subsection between the build-from-source block and `## Requirements`.

- [ ] **Step 2: Add this subsection immediately before `## Requirements`**

```markdown
### Onboarding skill

If you use Claude Code, install the bundled `/fleet` skill — it drives the whole first-run setup (prerequisites, relay, a tailored team, launch):

```bash
ln -s "$(pwd)/skill/fleet" ~/.claude/skills/fleet
```

Then in Claude Code say "set up fleet for this project" (or `/fleet`) and it walks you from zero to a running, registered fleet.
```

- [ ] **Step 3: Verify the subsection renders in context**

Run: `sed -n '/### Onboarding skill/,/## Requirements/p' README.md`
Expected: the new subsection followed by the `## Requirements` heading.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document the fleet onboarding skill install"
```

---

### Task 3: Validate skill correctness

**Files:**
- Review only: `skill/fleet/SKILL.md` against the real CLI and config rules.

- [ ] **Step 1: Verify every command in the skill exists in the CLI**

Run: `go build -o /tmp/fleet-skillcheck ./cmd/fleet && /tmp/fleet-skillcheck --help`
Expected: the help lists `dispatch`, `logs`, `usage`, and the flags `--doctor`, `--last`, `--status`. Confirm the skill uses only these (`fleet --doctor`, `fleet --last`, `fleet --status`, `fleet dispatch "<task>" --to <agent>`, `fleet logs <agent> -f`, `fleet usage`).

- [ ] **Step 2: Verify the config TOML shape matches the real format**

Run: `head -20 ~/.fleet/configs/*.toml 2>/dev/null | head -25`
Expected: a real saved config (if any) shows the same shape the skill writes — `[project]` (name, relay_url, cwd), `[claude]` flags, `[[agents]]` (name, color, role, is_executive/reports_to). If no config exists, confirm against `internal/config/config.go` that the struct tags match the keys used in the skill.

- [ ] **Step 3: Verify the validation rules quoted in the skill are accurate**

Run: `grep -n "validName\|unsafeRoleChars\|validColors" internal/config/config.go`
Expected: `validName` regex is `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`; `unsafeRoleChars` contains `" ' ` $ \` plus `\n\r\t`; `validColors` are green/orange/blue/red/purple/pink/cyan/yellow. Fix the skill text if any rule drifted.

- [ ] **Step 4: Self-review against the spec**

Re-read `docs/superpowers/specs/2026-06-11-fleet-onboarding-skill-design.md`. Confirm all seven flow steps, the consent gate, the launch mechanism, and the scope boundaries are reflected in the skill. Fix any gap inline.

- [ ] **Step 5: Commit any fixes**

```bash
git add skill/fleet/SKILL.md README.md
git commit -m "fix: correct fleet skill command/config details" || echo "no fixes needed"
```

---

### Task 4 (optional): Manual dry run

**Files:** none — runtime validation only.

- [ ] **Step 1: Dry-run the authored-config launch path on a throwaway project**

On a small scratch repo, follow the skill manually: write a 2-agent config to `~/.fleet/last.toml`, run `fleet --last`, confirm `fleet --status` shows both agents registered, then `fleet --kill` to clean up. This mirrors the relay e2e already used in this codebase. Skip if a live fleet launch is not desired right now — Tasks 1–3 are sufficient to ship the skill.
