package wizard

import "github.com/zairedegrees/fleet/internal/config"

// Preset represents a pre-defined team configuration.
type Preset struct {
	Name   string
	Icon   string
	Agents []config.AgentConfig
}

// palette for cyclic color assignment.
var palette = []string{"green", "orange", "blue", "red", "purple", "pink", "cyan", "yellow"}

// panel identifies which panel is active.
type panel int

const (
	panelLeft panel = iota
	panelRight
)

// leftFocus identifies which element has focus in the left panel.
type leftFocus int

const (
	focusName leftFocus = iota
	focusPath
	focusPresets
)

// drawerMode identifies what the bottom drawer is doing.
type drawerMode int

const (
	drawerEdit drawerMode = iota
	drawerCreate
)

// AllPresets returns the 10 built-in team presets. Each agent is behaviorally
// tuned (model + persona + skills + tool scope + permission posture) via
// config.RoleAgent; presets deviate from a role's default with config.WithModel/
// WithPerm/AsExecutive. Agents default to AutoTalk=false (stay idle until
// dispatched) to honor the fleet's token discipline — opt an agent into talk.
func AllPresets() []Preset {
	return []Preset{
		{
			Name: "Web App",
			Icon: "🌐",
			Agents: []config.AgentConfig{
				config.RoleAgent("dev", "green", ""),
				config.RoleAgent("frontend", "orange", "dev"),
				config.RoleAgent("ux-designer", "blue", "dev"),
				config.RoleAgent("auditor", "red", "dev"),
				config.RoleAgent("ops", "purple", "dev"),
			},
		},
		{
			Name: "API / Backend",
			Icon: "⚙",
			Agents: []config.AgentConfig{
				// Sole lead judgement carries the design — give dev Opus here.
				config.WithModel(config.RoleAgent("dev", "green", ""), "opus"),
				config.RoleAgent("auditor", "orange", "dev"),
				config.RoleAgent("ops", "blue", "dev"),
			},
		},
		{
			Name: "Data / ML",
			Icon: "📊",
			Agents: []config.AgentConfig{
				config.RoleAgent("dev", "green", ""),
				config.RoleAgent("researcher", "orange", "dev"),
				config.RoleAgent("quant", "blue", "dev"),
				config.RoleAgent("auditor", "red", "dev"),
			},
		},
		{
			Name: "Trading Bot",
			Icon: "💰",
			Agents: []config.AgentConfig{
				config.RoleAgent("dev", "green", ""),
				// Strategy work, but no live endpoints from this seat → default posture.
				config.WithPerm(config.RoleAgent("quant", "orange", "dev"), "default"),
				config.RoleAgent("auditor", "blue", "dev"),
				config.RoleAgent("researcher", "purple", "dev"),
				config.RoleAgent("ops", "red", "dev"),
				config.RoleAgent("notifier", "pink", "dev"),
			},
		},
		{
			Name: "Full Stack",
			Icon: "🚀",
			Agents: []config.AgentConfig{
				config.RoleAgent("dev", "green", ""),
				config.RoleAgent("frontend", "orange", "dev"),
				config.RoleAgent("ux-designer", "blue", "dev"),
				config.RoleAgent("auditor", "red", "dev"),
				config.RoleAgent("ops", "purple", "dev"),
				config.RoleAgent("researcher", "pink", "dev"),
				config.RoleAgent("docs", "cyan", "dev"),
			},
		},
		{
			Name: "Minimal",
			Icon: "⚡",
			Agents: []config.AgentConfig{
				// Tiny but high-quality: an Opus lead and an Opus reviewer.
				config.WithModel(config.RoleAgent("dev", "green", ""), "opus"),
				config.RoleAgent("auditor", "orange", "dev"),
			},
		},
		{
			// Solo Pair: cheap hands, expensive eyes — the new go-to default.
			Name: "Solo Pair",
			Icon: "⚡⚡",
			Agents: []config.AgentConfig{
				config.RoleAgent("dev", "green", ""),
				config.RoleAgent("auditor", "orange", "dev"),
			},
		},
		{
			// Design Studio: design leads under an architect (hierarchy flipped).
			Name: "Design Studio",
			Icon: "🎨",
			Agents: []config.AgentConfig{
				config.AsExecutive(config.RoleAgent("architect", "green", "")),
				config.RoleAgent("ux-designer", "orange", "architect"),
				config.RoleAgent("frontend", "blue", "architect"),
				config.RoleAgent("auditor", "red", "architect"),
			},
		},
		{
			// Security Hardening: read-mostly — only dev can write.
			Name: "Security Hardening",
			Icon: "🛡",
			Agents: []config.AgentConfig{
				config.AsExecutive(config.RoleAgent("architect", "green", "")),
				config.RoleAgent("security", "orange", "architect"),
				config.RoleAgent("auditor", "blue", "architect"),
				config.RoleAgent("dev", "red", "architect"),
			},
		},
		{
			Name:   "Custom",
			Icon:   "🔧",
			Agents: []config.AgentConfig{},
		},
	}
}

// GetPreset returns a preset by name, or nil if not found.
func GetPreset(name string) *Preset {
	for _, p := range AllPresets() {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// PresetAgentItems converts a preset's agents to agentItems (all enabled).
func PresetAgentItems(p Preset) []agentItem {
	items := make([]agentItem, len(p.Agents))
	for i, a := range p.Agents {
		items[i] = agentItem{agent: a, enabled: true}
	}
	return items
}
