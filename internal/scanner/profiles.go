package scanner

import "github.com/nazaire/fleet/internal/config"

// AgentSuggestion is a proposed agent for the fleet.
type AgentSuggestion struct {
	Agent  config.AgentConfig
	Reason string // why this agent is suggested
}

// palette assigns colors cyclically to avoid duplicates.
var palette = []string{"green", "orange", "blue", "red", "purple", "pink", "cyan", "yellow"}

// SuggestAgents proposes agents based on scan results.
func SuggestAgents(scan *ScanResult) []AgentSuggestion {
	var suggestions []AgentSuggestion
	colorIdx := 0

	nextColor := func() string {
		c := palette[colorIdx%len(palette)]
		colorIdx++
		return c
	}

	// Dev agent — always present
	suggestions = append(suggestions, AgentSuggestion{
		Agent: config.AgentConfig{
			Name:  "dev",
			Color: nextColor(),
			Role:  "Lead developer",
		},
		Reason: "Every project needs a dev agent",
	})

	// Auditor — if tests exist
	if scan.HasTests {
		suggestions = append(suggestions, AgentSuggestion{
			Agent: config.AgentConfig{
				Name:      "auditor",
				Color:     nextColor(),
				Role:      "Code review and testing",
				ReportsTo: "dev",
			},
			Reason: "Test directory detected",
		})
	}

	// Frontend — if frontend framework detected
	hasFrontend := false
	for _, fw := range scan.Frameworks {
		switch fw {
		case "React", "Vue", "Svelte", "Angular", "Next.js", "Nuxt", "Remix", "Astro":
			hasFrontend = true
		}
	}
	if hasFrontend {
		suggestions = append(suggestions, AgentSuggestion{
			Agent: config.AgentConfig{
				Name:      "frontend",
				Color:     nextColor(),
				Role:      "Frontend development",
				ReportsTo: "dev",
			},
			Reason: "Frontend framework detected",
		})
		suggestions = append(suggestions, AgentSuggestion{
			Agent: config.AgentConfig{
				Name:      "ux-designer",
				Color:     nextColor(),
				Role:      "UX design and user experience",
				ReportsTo: "dev",
			},
			Reason: "Frontend framework detected",
		})
	}

	// Ops — if infra detected
	if scan.HasInfra {
		suggestions = append(suggestions, AgentSuggestion{
			Agent: config.AgentConfig{
				Name:      "ops",
				Color:     nextColor(),
				Role:      "CI/CD and deployment",
				ReportsTo: "dev",
			},
			Reason: "Infrastructure files detected",
		})
	}

	// Researcher — if data/ML detected
	if scan.HasData {
		suggestions = append(suggestions, AgentSuggestion{
			Agent: config.AgentConfig{
				Name:      "researcher",
				Color:     nextColor(),
				Role:      "Data analysis and research",
				ReportsTo: "dev",
			},
			Reason: "Data/ML directory detected",
		})
	}

	// Architect — if monorepo
	if scan.IsMonorepo {
		suggestions = append(suggestions, AgentSuggestion{
			Agent: config.AgentConfig{
				Name:      "architect",
				Color:     nextColor(),
				Role:      "System architecture and design",
				ReportsTo: "dev",
			},
			Reason: "Monorepo structure detected (3+ packages)",
		})
	}

	// Quant — if finance
	if scan.IsFinance {
		suggestions = append(suggestions, AgentSuggestion{
			Agent: config.AgentConfig{
				Name:      "quant",
				Color:     nextColor(),
				Role:      "Quantitative analysis and trading strategy",
				ReportsTo: "dev",
			},
			Reason: "Finance/trading files detected",
		})
	}

	// Docs — if significant docs
	if scan.HasDocs {
		suggestions = append(suggestions, AgentSuggestion{
			Agent: config.AgentConfig{
				Name:      "docs",
				Color:     nextColor(),
				Role:      "Documentation",
				ReportsTo: "dev",
			},
			Reason: "Documentation directory detected",
		})
	}

	return suggestions
}
