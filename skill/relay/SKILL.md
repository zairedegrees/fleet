---
name: relay
description: Coordinate with other agents through fleet's embedded coordination core. Use for /relay, checking your inbox, messaging or dispatching work to other agents, tracking tasks, sharing memory, and running the autonomous talk loop. Triggers on /relay and multi-agent coordination.
---

# Relay — multi-agent coordination

You talk to other agents through an MCP server reachable as the `agent-relay`
tools (`get_inbox`, `send_message`, `dispatch_task`, ...). Under fleet this is the
embedded coordination core; the tools and wire are the same either way.

## Bootstrap

Are the `agent-relay` tools available (e.g. `get_inbox`, `register_agent`)?

- **No:** ensure `.mcp.json` in the project root has:
  ```json
  { "mcpServers": { "agent-relay": { "type": "http", "url": "http://localhost:8090/mcp" } } }
  ```
  then ask the user to run `/mcp` to reload. Stop here.
- **Yes:** continue.

## Identity

**Under fleet you are already registered** — your name, project, role and
profile were set when fleet launched you. Do **not** call `register_agent`
yourself (a bare re-register can disturb your server-side identity). Instead pass
your identity explicitly on every call:

- `as: "<your-name>"` — who you act as
- `project: "<your-project>"` — your project namespace

```
get_inbox(as: "backend", project: "my-app")
send_message(as: "backend", project: "my-app", to: "frontend", subject: "API ready", content: "...")
```

If you are running standalone (not under fleet) and the tools show you
unregistered, call `register_agent(name, project, role)` once. Calling it again
for the same name is a respawn: anything you leave out — `profile_slug`,
`reports_to`, `is_executive`, `session_id` — keeps its previous value, and only
the fields you pass (such as role) change. So a second register can never wipe
identity an orchestrator set for you; pass a field explicitly when you do want to
change it.

## Commands

### `inbox` (default)
`get_inbox(as, project, unread_only: true)` → show each message (from, subject,
priority, body) → after acting, `mark_read(as, project, message_ids: [...])` so
they leave your inbox.

### `send <agent> <message>`
`send_message(as, project, to: "<agent>", subject: "...", content: "...")`. Use
`to: "*"` to broadcast. Priority `P0` (interrupt) … `P3` (info), default `P2`.

### `conversations` — multi-turn threads
For a back-and-forth (e.g. a review discussion), keep it in one thread instead
of loose messages:
- `start_conversation(as, project, subject, [to], [content])` → opens a named
  thread and returns its `id`; pass `to`+`content` to post the first message in
  one call.
- Reply with `send_message(..., conversation_id: "<id>")`.
- `get_conversation(project, conversation_id, [limit], [before])` → pull the
  thread (recent messages, chronological) when you need the full context.
- `list_conversations(as, project)` → the threads you're in, with unread counts.

Thread messages still land in your `inbox` with their `conversation_id` — fetch
the whole thread only when you need the broader context (token discipline).

### `agents`
`list_agents(project)` → table of name / role / status.

### `tasks`
- `list_tasks(project, status: "active")` — non-done/cancelled work.
- Claim and work: `claim_task` → `start_task` → `complete_task(result: "...")`.
  Stuck? `block_task(reason: "...")`. Inspect with `get_task(task_id)`.
- Delegate: `dispatch_task(project, profile: "<profile>", title, description,
  priority)` — routes to agents on that profile and notifies them.

### `memory`
- `set_memory(project, key, value, scope)` — `scope` is `agent` | `project` |
  `global` (default `project`). A changed value versions the old one.
- `get_memory(project, key)` — no scope cascades agent → project → global.

### `talk` — autonomous loop
Proactively coordinate until things go quiet:

1. `get_inbox(as, project, unread_only: true)`.
2. For each message: act on it (answer questions, pick up tasks, update memory),
   reply with `send_message`, then `mark_read`.
3. Advance your tasks (`claim`/`start`/`complete`).
4. If the inbox was empty, count it. After **3 consecutive empty checks**, stop
   and report a short summary. Otherwise repeat from step 1.

Keep replies focused and always carry `as` + `project`.
