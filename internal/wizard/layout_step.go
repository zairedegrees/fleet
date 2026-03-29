package wizard

import "math"

// autoLayout calculates the grid dimensions (cols x rows).
// Single row up to 4, then 2 rows up to 8, capped at 4 columns.
func autoLayout(agentCount int) (cols, rows int) {
	if agentCount <= 0 {
		return 0, 0
	}
	if agentCount <= 4 {
		return agentCount, 1
	}
	cols = int(math.Ceil(float64(agentCount) / 2.0))
	if cols > 4 {
		cols = 4
	}
	rows = int(math.Ceil(float64(agentCount) / float64(cols)))
	return
}
