package coord

// toolDef is one entry of the MCP tools/list response: a tool name, a
// description the agent's model reads, and a JSON Schema for its arguments.
type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func numProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// asProp / projectProp appear on most tools: they override the identity/project
// otherwise implied by the connection.
var asProp = strProp("Act as this agent (overrides the default identity). Use when managing multiple agents from one session.")
var projectProp = strProp("Project namespace (overrides the default). Agents, tasks and messages are isolated per project.")

// toolDefs is the ordered tool catalog coord advertises via tools/list. It is the
// subset of the wrai.th relay surface that fleet and its agents actually use.
var toolDefs = []toolDef{
	{"whoami", "Identify your Claude Code session. Generate a unique salt (3+ random words), write it in your message, then call this with that salt; the relay finds your session_id by searching ~/.claude transcripts. Use the returned session_id in register_agent.",
		schema(map[string]any{"salt": strProp("A unique 3+ random-word string present in your transcript (e.g. 'purple-falcon-nebula').")}, "salt")},

	{"register_agent", "Register (or respawn) an agent. Call once at startup. Re-registering the same name+project updates role/description but PRESERVES omitted identity fields (reports_to, profile_slug, is_executive, session_id). is_executive=true auto-creates the 'leadership' admin team and enables broadcast.",
		schema(map[string]any{
			"project":           projectProp,
			"name":              strProp("Unique agent name (e.g. 'lead', 'backend')."),
			"role":              strProp("Agent role description."),
			"description":       strProp("What this agent is currently working on."),
			"reports_to":        strProp("Name of the agent this one reports to."),
			"is_executive":      boolProp("Mark as executive (auto-joins the leadership admin team)."),
			"profile_slug":      strProp("Profile archetype this agent runs."),
			"session_id":        strProp("Claude Code session ID for activity tracking."),
			"interest_tags":     strProp("JSON array of interest tags for context budgeting."),
			"max_context_bytes": numProp("Max bytes for budget-pruned inbox (default 16384)."),
		}, "name")},

	{"register_profile", "Create or update a profile (a reusable agent archetype). Upserts on (project, slug); preserves created_at.",
		schema(map[string]any{
			"project":       projectProp,
			"slug":          strProp("Unique profile slug."),
			"name":          strProp("Human-readable profile name."),
			"role":          strProp("Profile role description."),
			"context_pack":  strProp("Context pack text injected for agents on this profile."),
			"soul_keys":     strProp("JSON array of soul keys."),
			"skills":        strProp("JSON array of skills."),
			"vault_paths":   strProp("JSON array of vault path globs."),
			"allowed_tools": strProp("JSON array of allowed tool patterns."),
			"pool_size":     numProp("Max concurrent spawns for this profile (default 3)."),
		}, "slug", "name")},

	{"list_agents", "List the agents registered in a project (active, sleeping or inactive), ordered by name.",
		schema(map[string]any{"project": projectProp})},

	{"deactivate_agent", "Deactivate an agent so it no longer appears in routing. Reports success even if the agent did not exist.",
		schema(map[string]any{"project": projectProp, "name": strProp("Agent name to deactivate.")}, "name")},

	{"dispatch_task", "Dispatch a task to a profile. Routes to agents running that profile and notifies them. Priority is P0-P3 (default P2).",
		schema(map[string]any{
			"as": asProp, "project": projectProp,
			"profile":     strProp("Profile slug to route the task to."),
			"title":       strProp("Short task title."),
			"description": strProp("What to do and acceptance criteria."),
			"priority":    strProp("P0 (interrupt) .. P3 (info). Default P2."),
			"goal_id":     strProp("Optional: attach this task to a goal (must exist; see create_goal)."),
		}, "profile", "title")},

	{"list_tasks", "List tasks in a project, ordered by priority then recency. status='active' excludes done/cancelled. count reflects the returned page.",
		schema(map[string]any{
			"project":     projectProp,
			"profile":     strProp("Filter by profile slug."),
			"status":      strProp("Filter by status, or 'active' for non-terminal."),
			"priority":    strProp("Filter by priority (P0-P3)."),
			"assigned_to": strProp("Filter by assigned agent."),
			"limit":       numProp("Max tasks to return (default 50)."),
		})},

	{"get_task", "Get a single task by id (or a unique id prefix), with full description/result.",
		schema(map[string]any{"project": projectProp, "task_id": strProp("Task id or a unique prefix.")}, "task_id")},

	{"claim_task", "Claim a pending task (assigns it to you, status -> accepted).",
		schema(map[string]any{"as": asProp, "project": projectProp, "task_id": strProp("Task id or unique prefix.")}, "task_id")},

	{"start_task", "Start working on a task (status -> in-progress).",
		schema(map[string]any{"as": asProp, "project": projectProp, "task_id": strProp("Task id or unique prefix.")}, "task_id")},

	{"complete_task", "Complete a task (status -> done). Optionally attach a result.",
		schema(map[string]any{"as": asProp, "project": projectProp, "task_id": strProp("Task id or unique prefix."), "result": strProp("Result/summary (string or JSON).")}, "task_id")},

	{"block_task", "Block an in-progress task (status -> blocked) with a reason.",
		schema(map[string]any{"as": asProp, "project": projectProp, "task_id": strProp("Task id or unique prefix."), "reason": strProp("Why it is blocked.")}, "task_id")},

	{"send_message", "Send a message to another agent. Use '*' to broadcast. Priority accepts P0-P3 or aliases (interrupt/steering/advisory/info).",
		schema(map[string]any{
			"as": asProp, "project": projectProp,
			"to":       strProp("Recipient agent name, or '*' for broadcast."),
			"subject":  strProp("Message subject."),
			"content":  strProp("Message body."),
			"type":     strProp("Message type (default 'notification')."),
			"priority": strProp("P0-P3 or an alias. Default P2."),
			"reply_to": strProp("Message id to reply to (threading)."),
			"metadata": strProp("JSON string of extra metadata."),
		}, "to", "content")},

	{"get_inbox", "Get your pending messages (queued/surfaced), ordered by priority then recency. Reading surfaces them. unread_only defaults true; content truncates to 300 chars unless full_content.",
		schema(map[string]any{
			"as": asProp, "project": projectProp,
			"unread_only":  boolProp("Only unread (default true)."),
			"limit":        numProp("Max messages (default 10)."),
			"full_content": boolProp("Return full content instead of truncating (default false)."),
		})},

	{"mark_read", "Mark messages read (acknowledges their delivery so they leave your inbox). Counts only newly-read ids.",
		schema(map[string]any{"as": asProp, "project": projectProp, "message_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Message ids to mark read."}}, "message_ids")},

	{"set_memory", "Store a shared memory under a key. scope is agent/project/global (default project). A changed value versions (archives the old); upsert=false flags a conflict instead.",
		schema(map[string]any{
			"as": asProp, "project": projectProp,
			"key":        strProp("Memory key."),
			"value":      strProp("Memory value (string or JSON)."),
			"scope":      strProp("agent | project | global (default project)."),
			"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags for filtering."},
			"confidence": strProp("stated | inferred | observed (default stated)."),
			"layer":      strProp("constraints | behavior | context (default behavior)."),
			"upsert":     boolProp("Overwrite (default true) vs flag a conflict (false)."),
		}, "key", "value")},

	{"get_memory", "Get a memory by key. With no scope, cascades agent -> project -> global (first match wins). Multiple active values surface a conflict.",
		schema(map[string]any{"as": asProp, "project": projectProp, "key": strProp("Memory key."), "scope": strProp("agent | project | global (optional).")}, "key")},

	{"get_session_context", "Get your full working context: profile, pending tasks (assigned + dispatched), unread messages, and relevant memories.",
		schema(map[string]any{"as": asProp, "project": projectProp, "profile_slug": strProp("Profile to load (auto-detected from your registration if omitted).")})},

	{"list_orgs", "List organizations. Used as the relay health probe.",
		schema(map[string]any{})},

	{"start_conversation", "Start a named conversation thread and get its id. Pass `to` and `content` to also post the opening message in one call. Reply later with send_message(conversation_id=...).",
		schema(map[string]any{
			"project":  projectProp,
			"as":       asProp,
			"subject":  strProp("Short thread subject."),
			"to":       strProp("Optional: recipient of an opening message ('*' broadcasts)."),
			"content":  strProp("Optional: opening message body (requires `to`)."),
			"priority": strProp("Optional opening-message priority P0-P3 (default P2)."),
		}, "subject")},

	{"get_conversation", "Get a conversation thread: metadata + messages in chronological order. Returns the most recent `limit` messages; page older with `before` (a created_at cursor). Content is truncated unless full_content=true.",
		schema(map[string]any{
			"project":         projectProp,
			"conversation_id": strProp("The conversation id."),
			"limit":           numProp("Max messages to return (default 20)."),
			"before":          strProp("Cursor: return messages created before this value (page older)."),
			"full_content":    boolProp("Return full bodies instead of 300-char previews."),
		}, "conversation_id")},

	{"list_conversations", "List the conversations you're part of (started, or sent/received a message in), most recent first. Compact summaries only (subject, counts) — no bodies; use get_conversation for the thread.",
		schema(map[string]any{
			"project": projectProp,
			"as":      asProp,
			"status":  strProp("Optional: filter by status (e.g. 'open')."),
			"limit":   numProp("Max conversations to return (default 20)."),
		})},

	{"create_goal", "Create a high-level goal that groups tasks. Dispatch tasks under it with dispatch_task(goal_id=...), then track progress with get_goal / list_goals.",
		schema(map[string]any{
			"project":     projectProp,
			"as":          asProp,
			"title":       strProp("Short goal title."),
			"description": strProp("Optional: what the goal is about."),
		}, "title")},

	{"get_goal", "Get a goal and its derived progress (task counts: total, done, in_progress, blocked). Use list_tasks(goal_id=...) for the tasks themselves.",
		schema(map[string]any{
			"project": projectProp,
			"goal_id": strProp("The goal id."),
		}, "goal_id")},

	{"list_goals", "List goals in a project, most recent first, each with done/total task progress. Compact — no descriptions; use get_goal for one goal's full progress.",
		schema(map[string]any{
			"project": projectProp,
			"status":  strProp("Optional: filter by status (e.g. 'open')."),
			"limit":   numProp("Max goals to return (default 50)."),
		})},
}

// operatorOnly tools are handled on tools/call (the fleet CLI invokes them by
// name) but NOT advertised on tools/list. Agents never call them — fleet
// registers agents and profiles server-side and drives orchestration — so
// keeping them out of every agent's catalog trims ~780 tokens of context per
// agent. Dropping register_agent also enforces the no-self-register design: an
// agent can't call a tool it never sees.
var operatorOnly = map[string]bool{
	"register_agent":   true,
	"register_profile": true,
	"deactivate_agent": true,
	"list_orgs":        true,
}

// advertisedTools is the tools/list catalog: every toolDef except operator-only
// ones. tools/call still dispatches all handlers (see the handlers map).
func advertisedTools() []toolDef {
	out := make([]toolDef, 0, len(toolDefs))
	for _, t := range toolDefs {
		if !operatorOnly[t.Name] {
			out = append(out, t)
		}
	}
	return out
}
