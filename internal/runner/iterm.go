package runner

import (
	"fmt"
	"os"
	"strings"
)

func OpenITerm2Grid(project string, agents []string) error {
	if len(agents) == 0 {
		return nil
	}
	if !isITerm2Available() {
		fmt.Println("  iTerm2 not found. Attach manually:")
		for _, agent := range agents {
			fmt.Printf("    tmux attach -t %s\n", SessionName(project, agent))
		}
		return nil
	}

	script := buildAppleScript(project, agents)

	// Write to a known path so the user can re-run it
	scriptPath := fmt.Sprintf("%s/.fleet/iterm-grid.scpt", os.Getenv("HOME"))
	os.MkdirAll(fmt.Sprintf("%s/.fleet", os.Getenv("HOME")), 0755)
	os.WriteFile(scriptPath, []byte(script), 0755)

	// Try running it — may fail due to macOS Automation permissions
	cmd := execCommand("osascript", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Printf("\n  ⚠ iTerm2 grid failed (likely macOS Automation permission).\n")
		fmt.Printf("  Run manually: osascript ~/.fleet/iterm-grid.scpt\n\n")
	}
	return nil
}

func isITerm2Available() bool {
	_, err := os.Stat("/Applications/iTerm.app")
	return err == nil
}

// buildAppleScript reproduces the exact pattern from the working POC:
// 1. Create window
// 2. Split into 2 columns (split horizontally)
// 3. Split each column into rows (split vertically)
// 4. Write tmux attach to each pane
func buildAppleScript(project string, agents []string) string {
	n := len(agents)
	leftCount := (n + 1) / 2
	left := agents[:leftCount]
	right := agents[leftCount:]

	var sb strings.Builder

	sb.WriteString(`tell application "iTerm2"
    activate
    create window with default profile
    tell current window
        tell current session
            write text "tmux attach -t ` + SessionName(project, left[0]) + `"
        end tell
`)

	// --- Phase 1: Create all panes ---

	if len(right) > 0 {
		// Split into 2 columns
		sb.WriteString(`
        tell current session
            set rightPane to (split horizontally with default profile)
        end tell
`)
	}

	// Left column: create rows by splitting vertically
	// Split current session to create leftPane2
	// Split current session again to create leftPane3
	// Split leftPane2 to create leftPane4 (interleaved for even sizes)
	leftPanes := []string{"current session"} // leftPanes[0] = top of left column
	for i := 1; i < len(left); i++ {
		varName := fmt.Sprintf("leftPane%d", i+1)
		// Split from existing pane using round-robin for balanced sizing
		splitFrom := leftPanes[(i-1)/2]
		sb.WriteString(fmt.Sprintf(`
        tell %s
            set %s to (split vertically with default profile)
        end tell
`, splitFrom, varName))
		// Insert in order: we interleave to get balanced splits
		leftPanes = append(leftPanes, varName)
	}

	// Right column: create rows by splitting vertically
	rightPanes := []string{}
	if len(right) > 0 {
		rightPanes = append(rightPanes, "rightPane")
		for i := 1; i < len(right); i++ {
			varName := fmt.Sprintf("rightPane%d", i+1)
			splitFrom := rightPanes[(i-1)/2]
			sb.WriteString(fmt.Sprintf(`
        tell %s
            set %s to (split vertically with default profile)
        end tell
`, splitFrom, varName))
			rightPanes = append(rightPanes, varName)
		}
	}

	// --- Phase 2: Write tmux attach to each pane ---
	// Left column - reorder panes to match visual top-to-bottom
	leftOrdered := reorderPanes(leftPanes)
	for i, pane := range leftOrdered {
		if i == 0 && pane == "current session" {
			continue // already wrote tmux attach above
		}
		sb.WriteString(fmt.Sprintf(`
        tell %s
            write text "tmux attach -t %s"
        end tell
`, pane, SessionName(project, left[i])))
	}

	// Right column
	rightOrdered := reorderPanes(rightPanes)
	for i, pane := range rightOrdered {
		sb.WriteString(fmt.Sprintf(`
        tell %s
            write text "tmux attach -t %s"
        end tell
`, pane, SessionName(project, right[i])))
	}

	sb.WriteString(`
    end tell
end tell
`)
	return sb.String()
}

// reorderPanes reorders pane variable names from split-creation order
// to visual top-to-bottom order. When you split panes using round-robin,
// the visual order is: [0], [2], [4]..., [1], [3], [5]...
// For the simple case of the POC pattern, we just return as-is since
// the splits create panes in a reasonable visual order.
func reorderPanes(panes []string) []string {
	return panes
}
