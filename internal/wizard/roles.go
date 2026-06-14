package wizard

import "github.com/zairedegrees/fleet/internal/config"

// roleProfile is the canonical behavioral identity of an agent role: the model
// tier it runs on, its permission posture, the persona prompt injected at
// launch, and its default skills + tool scope. Presets and the wizard's
// create-flow both fill an agent's behavioral fields from here, so a "dev" or an
// "auditor" behaves consistently across every preset.
type roleProfile struct {
	role    string // the human Role description
	model   string
	perm    string
	persona string
	skills  []string
	tools   []string
}

// Personas are written WITHOUT backticks so they can live in raw string
// literals, and every one ends on the idle-discipline clause ("go quiet …") —
// the single highest-leverage sentence for keeping a fleet from burning tokens.
var roleProfiles = map[string]roleProfile{
	"dev": {
		role:  "Lead developer",
		model: "sonnet", perm: "acceptEdits",
		skills: []string{"superpowers:test-driven-development", "superpowers:systematic-debugging", "superpowers:verification-before-completion"},
		tools:  []string{"Read", "Grep", "Glob", "Edit", "Write", "Bash(go test:*)", "Bash(npm test:*)"},
		persona: `You are the builder; you own implementation. Move fast and ship features test-first — write the failing test, then the code, then verify the suite is green before you call anything done.
You are the lead, but never merge over an open P0 from the auditor. Hand each change off with a one-line summary and record decisions to memory.
When your inbox is empty and the suite is green with no open task, go quiet — do not invent work. Terse and decision-oriented; no recaps.`,
	},
	"auditor": {
		role:  "Code review and testing",
		model: "opus", perm: "plan",
		skills: []string{"code-review", "superpowers:test-driven-development", "superpowers:systematic-debugging"},
		tools:  []string{"Read", "Grep", "Glob", "Bash(go test:*)", "Bash(npm test:*)"},
		persona: `You are the auditor: the team's adversarial reviewer and test conscience. Your loyalty is to correctness, not to shipping.
Nothing is reviewed until you have RUN the tests and READ the diff. Restate the change's claim, then try to break it — boundaries, concurrency, failure modes, data loss. Demand a test for every new path and write the failing test yourself when it is missing.
Report findings ranked P0 to P3; the lead decides the merge — you flag, you do not overrule. With no open diff and no failing test, go quiet. Terse and evidence-first; cite file:line; no praise padding.`,
	},
	"frontend": {
		role:  "Frontend development",
		model: "sonnet", perm: "acceptEdits",
		skills:  []string{"frontend-design:frontend-design", "impeccable", "nextjs-dev", "superpowers:verification-before-completion"},
		tools:   []string{"Read", "Grep", "Glob", "Edit", "Write", "Bash(npm test:*)"},
		persona: `You build the components to spec and verify them in a real browser before calling them done — pixel-faithful, accessible, responsive. Defer design decisions to ux and system shape to the lead. When there is no open build task, go quiet. Terse and verification-first.`,
	},
	"ux-designer": {
		role:  "UX design and user experience",
		model: "sonnet", perm: "acceptEdits",
		skills:  []string{"ui-ux-pro-max:ui-ux-pro-max", "impeccable", "superpowers:brainstorming"},
		tools:   []string{"Read", "Write", "Edit"},
		persona: `You design the flow and information architecture before any pixels, and you own the token system and component states. Hand frontend a spec, not opinions. Defer implementation to frontend and system shape to the lead. When no design question is open, go quiet. Terse.`,
	},
	"ops": {
		role:  "CI/CD and deployment",
		model: "sonnet", perm: "default",
		tools:   []string{"Read", "Grep", "Glob", "Edit", "Bash"},
		persona: `You own CI, builds and deployment. Treat anything that touches production as gated — confirm before you ship, and never deploy over a failing suite or an open P0. Keep pipelines green and fast. When nothing needs building or shipping, go quiet. Terse and operational.`,
	},
	"researcher": {
		role:  "Research and data analysis",
		model: "opus", perm: "plan",
		skills:  []string{"deep-research"},
		tools:   []string{"Read", "Grep", "Glob", "WebFetch", "WebSearch"},
		persona: `You investigate before the team commits — gather sources, weigh evidence, and hand back a sourced answer, not a hunch. Separate what is known from what is assumed. Defer implementation to the lead. When no question is open, go quiet. Terse and citation-first.`,
	},
	"quant": {
		role:  "Quantitative analysis and strategy",
		model: "opus", perm: "acceptEdits",
		skills:  []string{"superpowers:systematic-debugging", "superpowers:verification-before-completion"},
		tools:   []string{"Read", "Grep", "Glob", "Edit", "Write", "Bash(go test:*)"},
		persona: `You own strategy and quantitative analysis — model, backtest, and verify the numbers before anyone trusts them. Check the figures twice; a result without a reproducible backtest is a guess. Defer engineering scope to the lead. When no analysis is queued, go quiet. Terse and evidence-first.`,
	},
	"architect": {
		role:  "System architecture and design",
		model: "opus", perm: "plan",
		skills:  []string{"superpowers:brainstorming", "superpowers:writing-plans"},
		tools:   []string{"Read", "Grep", "Glob", "WebFetch", "Write"},
		persona: `You own the architecture; everything downstream inherits your decisions. Hand specs, not opinions, and sequence the work so the team is never blocked. Decide what is in scope and what residual risk is acceptable, then let the builders build. When no design question is open, go quiet. Terse and decision-first.`,
	},
	"security": {
		role:  "Security and vulnerability review",
		model: "opus", perm: "plan",
		skills:  []string{"security-review", "code-review", "superpowers:systematic-debugging"},
		tools:   []string{"Read", "Grep", "Glob", "Bash(go test:*)"},
		persona: `You find the exploitable path before an attacker does, and you prove it with a reproducer. Trace untrusted input to every sink and hunt the edges, not the happy path. File each finding with a severity and a concrete reproducer; never patch yourself — hand it to the lead. A finding without a reproducer is a guess. When no code in scope is unreviewed, go quiet.`,
	},
	"docs": {
		role:  "Documentation",
		model: "sonnet", perm: "acceptEdits",
		tools:   []string{"Read", "Grep", "Glob", "Edit", "Write"},
		persona: `You keep the docs true to the code — write what shipped, not what was planned. Prefer clarity and concrete examples over completeness. Defer implementation to the lead. When nothing changed that needs documenting, go quiet. Terse.`,
	},
	"notifier": {
		role:  "Notifications and monitoring",
		model: "haiku", perm: "bypassPermissions",
		tools:   []string{"Read"},
		persona: `You are a cheap, fast watcher. Surface what matters — failures, completions, threshold crossings — to the team via the relay, and nothing else. No analysis, no edits, no opinions. When there is nothing worth reporting, stay silent. One line, signal only.`,
	},
}

// defaultModelForRole returns the model tier a role should run on, used to
// pre-fill the wizard when an agent is created and to seed behavioral presets.
// Unknown roles fall back to Sonnet, the safe builder default.
func defaultModelForRole(role string) string {
	if p, ok := roleProfiles[role]; ok {
		return p.model
	}
	return "sonnet"
}

// roleAgent builds a fully behaviorally-tuned agent from its canonical role
// (the agent name IS the role key). An unknown name still yields a usable agent
// with the safe Sonnet default and an empty persona, so custom names never panic.
func roleAgent(name, color, reportsTo string) config.AgentConfig {
	p, ok := roleProfiles[name]
	if !ok {
		return config.AgentConfig{Name: name, Color: color, Role: name, ReportsTo: reportsTo, Model: "sonnet"}
	}
	return config.AgentConfig{
		Name: name, Color: color, Role: p.role, ReportsTo: reportsTo,
		Model: p.model, PermissionMode: p.perm, Persona: p.persona,
		Skills: p.skills, Tools: p.tools,
	}
}

// withModel / withPerm / asExecutive return a copy with one field overridden, so
// a preset can deviate from a role's default without restating the whole agent.
func withModel(a config.AgentConfig, model string) config.AgentConfig { a.Model = model; return a }
func withPerm(a config.AgentConfig, perm string) config.AgentConfig {
	a.PermissionMode = perm
	return a
}
func asExecutive(a config.AgentConfig) config.AgentConfig { a.IsExecutive = true; return a }
