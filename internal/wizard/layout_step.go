package wizard

import "math"

// autoLayout calculates the grid dimensions (cols x rows) for a given
// number of agents, producing a roughly-square layout.
func autoLayout(agentCount int) (cols, rows int) {
	if agentCount <= 0 {
		return 0, 0
	}
	cols = int(math.Ceil(math.Sqrt(float64(agentCount))))
	rows = int(math.Ceil(float64(agentCount) / float64(cols)))
	return
}
