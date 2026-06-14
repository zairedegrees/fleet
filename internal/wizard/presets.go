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
// roleAgent; presets deviate from a role's default with withModel/withPerm/
// asExecutive. Agents default to AutoTalk=false (stay idle until dispatched) to
// honor the fleet's token discipline — the user opts an agent into the talk loop.
func AllPresets() []Preset {
	return []Preset{
		{
			Name: "Web App",
			Icon: "🌐",
			Agents: []config.AgentConfig{
				roleAgent("dev", "green", ""),
				roleAgent("frontend", "orange", "dev"),
				roleAgent("ux-designer", "blue", "dev"),
				roleAgent("auditor", "red", "dev"),
				roleAgent("ops", "purple", "dev"),
			},
		},
		{
			Name: "API / Backend",
			Icon: "⚙",
			Agents: []config.AgentConfig{
				// Sole lead judgement carries the design — give dev Opus here.
				withModel(roleAgent("dev", "green", ""), "opus"),
				roleAgent("auditor", "orange", "dev"),
				roleAgent("ops", "blue", "dev"),
			},
		},
		{
			Name: "Data / ML",
			Icon: "📊",
			Agents: []config.AgentConfig{
				roleAgent("dev", "green", ""),
				roleAgent("researcher", "orange", "dev"),
				roleAgent("quant", "blue", "dev"),
				roleAgent("auditor", "red", "dev"),
			},
		},
		{
			Name: "Trading Bot",
			Icon: "💰",
			Agents: []config.AgentConfig{
				roleAgent("dev", "green", ""),
				// Strategy work, but no live endpoints from this seat → default posture.
				withPerm(roleAgent("quant", "orange", "dev"), "default"),
				roleAgent("auditor", "blue", "dev"),
				roleAgent("researcher", "purple", "dev"),
				roleAgent("ops", "red", "dev"),
				roleAgent("notifier", "pink", "dev"),
			},
		},
		{
			Name: "Full Stack",
			Icon: "🚀",
			Agents: []config.AgentConfig{
				roleAgent("dev", "green", ""),
				roleAgent("frontend", "orange", "dev"),
				roleAgent("ux-designer", "blue", "dev"),
				roleAgent("auditor", "red", "dev"),
				roleAgent("ops", "purple", "dev"),
				roleAgent("researcher", "pink", "dev"),
				roleAgent("docs", "cyan", "dev"),
			},
		},
		{
			Name: "Minimal",
			Icon: "⚡",
			Agents: []config.AgentConfig{
				// Tiny but high-quality: an Opus lead and an Opus reviewer.
				withModel(roleAgent("dev", "green", ""), "opus"),
				roleAgent("auditor", "orange", "dev"),
			},
		},
		{
			// Solo Pair: cheap hands, expensive eyes — the new go-to default.
			Name: "Solo Pair",
			Icon: "⚡⚡",
			Agents: []config.AgentConfig{
				roleAgent("dev", "green", ""),
				roleAgent("auditor", "orange", "dev"),
			},
		},
		{
			// Design Studio: design leads under an architect (hierarchy flipped).
			Name: "Design Studio",
			Icon: "🎨",
			Agents: []config.AgentConfig{
				asExecutive(roleAgent("architect", "green", "")),
				roleAgent("ux-designer", "orange", "architect"),
				roleAgent("frontend", "blue", "architect"),
				roleAgent("auditor", "red", "architect"),
			},
		},
		{
			// Security Hardening: read-mostly — only dev can write.
			Name: "Security Hardening",
			Icon: "🛡",
			Agents: []config.AgentConfig{
				asExecutive(roleAgent("architect", "green", "")),
				roleAgent("security", "orange", "architect"),
				roleAgent("auditor", "blue", "architect"),
				roleAgent("dev", "red", "architect"),
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
