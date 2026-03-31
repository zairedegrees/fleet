package wizard

import (
	"strings"

	"github.com/nazaire/fleet/internal/config"
)

// agentColors is the palette for agent color assignment.
var agentColors = []string{
	"green", "orange", "blue", "red", "purple", "pink", "cyan", "yellow",
}

// agentItem represents a selectable agent in the list.
type agentItem struct {
	agent   config.AgentConfig
	enabled bool
}

// normalizeName converts user input to a valid agent name.
// "UX Designer" → "ux-designer", "My Agent!" → "my-agent"
func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		}
	}
	return string(result)
}
