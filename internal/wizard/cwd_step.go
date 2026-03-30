package wizard

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nazaire/fleet/internal/config"
)

func runCwdStep(project string) (string, error) {
	// Try to load from saved config first
	configPath := filepath.Join(config.FleetDir(), "configs", project+".toml")
	if cfg, err := config.Load(configPath); err == nil && cfg.Project.Cwd != "" {
		fmt.Printf("%s %s\n", titleStyle.Render("Project directory"), selectedStyle.Render(cfg.Project.Cwd))
		fmt.Printf("  %s\n\n", dimStyle.Render("(from saved config — edit in ~/.fleet/configs/"+project+".toml)"))
		return cfg.Project.Cwd, nil
	}

	// No saved config — ask with native shell readline (tab completion)
	cwd, _ := os.Getwd()
	fmt.Printf("%s\n", titleStyle.Render("Project directory"))
	fmt.Printf("  %s\n\n", dimStyle.Render("Tab completion available. Enter to confirm."))

	// Use zsh with vared for readline + tab completion (macOS default shell)
	cmd := exec.Command("zsh", "-c",
		fmt.Sprintf(`path="%s"; vared -p "  Path: " path && echo "$path"`, cwd))
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		// Fallback to basic input if bash readline fails
		fmt.Printf("  Path [%s]: ", cwd)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text != "" {
				return expandHome(text), nil
			}
		}
		return cwd, nil
	}

	result := strings.TrimSpace(string(out))
	if result == "" {
		return cwd, nil
	}
	return expandHome(result), nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
