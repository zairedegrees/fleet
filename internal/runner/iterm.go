package runner

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
)

// OpenITerm2Grid opens a grid of iTerm2 panes, each attached to a tmux session
// for the given agents. If iTerm2 is not available, fallback tmux attach
// commands are printed instead.
func OpenITerm2Grid(agents []string) error {
	if len(agents) == 0 {
		return nil
	}

	if !isITerm2Available() {
		fmt.Println("iTerm2 not found. To attach manually, run:")
		for _, agent := range agents {
			fmt.Printf("  tmux attach -t %s\n", SessionName(agent))
		}
		return nil
	}

	cols, rows := gridSize(len(agents))
	script := buildAppleScript(agents, cols, rows)

	cmd := exec.Command("osascript", "-e", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isITerm2Available returns true if iTerm2 is installed at the standard path.
func isITerm2Available() bool {
	_, err := os.Stat("/Applications/iTerm.app")
	return err == nil
}

// gridSize calculates the number of columns and rows for a grid of n panes.
func gridSize(n int) (cols, rows int) {
	cols = int(math.Ceil(math.Sqrt(float64(n))))
	rows = int(math.Ceil(float64(n) / float64(cols)))
	return
}

// buildAppleScript generates the AppleScript that creates the iTerm2 grid.
func buildAppleScript(agents []string, cols, rows int) string {
	var sb strings.Builder

	sb.WriteString("tell application \"iTerm2\"\n")
	sb.WriteString("  activate\n")
	sb.WriteString("  set newWindow to (create window with default profile)\n")
	sb.WriteString("  tell newWindow\n")
	sb.WriteString("    tell current session\n")

	// First pane: attach to the first agent's session
	firstSession := SessionName(agents[0])
	sb.WriteString(fmt.Sprintf("      write text \"tmux attach -t %s\"\n", firstSession))

	// Build pane index map: row-major order
	idx := 1 // agents[0] is already in place

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			if row == 0 && col == 0 {
				continue // first pane already created
			}
			if idx >= len(agents) {
				break
			}

			session := SessionName(agents[idx])

			if col == 0 {
				// Start a new row: split the first pane of the previous row horizontally
				sb.WriteString("    end tell\n")
				sb.WriteString("    set rowAnchor to first session\n")
				sb.WriteString("    tell rowAnchor\n")
				sb.WriteString(fmt.Sprintf("      set newPane to (split horizontally with default profile)\n"))
				sb.WriteString("    end tell\n")
				sb.WriteString("    tell newPane\n")
				sb.WriteString(fmt.Sprintf("      write text \"tmux attach -t %s\"\n", session))
			} else {
				// Same row: split the previous pane vertically
				sb.WriteString("    end tell\n")
				sb.WriteString("    tell (last session)\n")
				sb.WriteString(fmt.Sprintf("      set newPane to (split vertically with default profile)\n"))
				sb.WriteString("    end tell\n")
				sb.WriteString("    tell newPane\n")
				sb.WriteString(fmt.Sprintf("      write text \"tmux attach -t %s\"\n", session))
			}

			idx++
		}
	}

	sb.WriteString("    end tell\n")
	sb.WriteString("  end tell\n")
	sb.WriteString("end tell\n")

	return sb.String()
}
