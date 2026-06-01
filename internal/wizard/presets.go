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

// AllPresets returns the 7 built-in team presets.
func AllPresets() []Preset {
	return []Preset{
		{
			Name: "Web App",
			Icon: "🌐",
			Agents: []config.AgentConfig{
				{Name: "dev", Color: "green", Role: "Lead developer"},
				{Name: "frontend", Color: "orange", Role: "Frontend development"},
				{Name: "ux-designer", Color: "blue", Role: "UX design and user experience", ReportsTo: "dev"},
				{Name: "auditor", Color: "red", Role: "Code review and testing", ReportsTo: "dev"},
				{Name: "ops", Color: "purple", Role: "CI/CD and deployment", ReportsTo: "dev"},
			},
		},
		{
			Name: "API / Backend",
			Icon: "⚙",
			Agents: []config.AgentConfig{
				{Name: "dev", Color: "green", Role: "Lead developer"},
				{Name: "auditor", Color: "orange", Role: "Code review and testing", ReportsTo: "dev"},
				{Name: "ops", Color: "blue", Role: "CI/CD and deployment", ReportsTo: "dev"},
			},
		},
		{
			Name: "Data / ML",
			Icon: "📊",
			Agents: []config.AgentConfig{
				{Name: "dev", Color: "green", Role: "Lead developer"},
				{Name: "researcher", Color: "orange", Role: "Data analysis and research", ReportsTo: "dev"},
				{Name: "quant", Color: "blue", Role: "Quantitative analysis", ReportsTo: "dev"},
				{Name: "auditor", Color: "red", Role: "Code review and testing", ReportsTo: "dev"},
			},
		},
		{
			Name: "Trading Bot",
			Icon: "💰",
			Agents: []config.AgentConfig{
				{Name: "dev", Color: "green", Role: "Lead developer"},
				{Name: "quant", Color: "orange", Role: "Quantitative analysis and trading strategy", ReportsTo: "dev"},
				{Name: "auditor", Color: "blue", Role: "Code review and testing", ReportsTo: "dev"},
				{Name: "ops", Color: "red", Role: "CI/CD and deployment", ReportsTo: "dev"},
				{Name: "researcher", Color: "purple", Role: "Market research and data analysis", ReportsTo: "dev"},
				{Name: "ux-designer", Color: "pink", Role: "UX design and notifications", ReportsTo: "dev"},
			},
		},
		{
			Name: "Full Stack",
			Icon: "🚀",
			Agents: []config.AgentConfig{
				{Name: "dev", Color: "green", Role: "Lead developer"},
				{Name: "frontend", Color: "orange", Role: "Frontend development", ReportsTo: "dev"},
				{Name: "ux-designer", Color: "blue", Role: "UX design and user experience", ReportsTo: "dev"},
				{Name: "auditor", Color: "red", Role: "Code review and testing", ReportsTo: "dev"},
				{Name: "ops", Color: "purple", Role: "CI/CD and deployment", ReportsTo: "dev"},
				{Name: "researcher", Color: "pink", Role: "Research and documentation", ReportsTo: "dev"},
				{Name: "docs", Color: "cyan", Role: "Documentation", ReportsTo: "dev"},
			},
		},
		{
			Name: "Minimal",
			Icon: "⚡",
			Agents: []config.AgentConfig{
				{Name: "dev", Color: "green", Role: "Lead developer"},
				{Name: "auditor", Color: "orange", Role: "Code review and testing", ReportsTo: "dev"},
			},
		},
		{
			Name: "Custom",
			Icon: "🔧",
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
