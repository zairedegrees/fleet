package runner

import (
	"slices"
	"strings"

	"github.com/zairedegrees/fleet/internal/config"
)

// toolsValue builds the --allowedTools value from an agent's allow-list. It is
// comma-joined (not space-joined) so a single shell-quoted value can carry
// specifiers with inner spaces like "Bash(go test:*)", and it ALWAYS includes
// the relay rule: --allowedTools switches Claude to allow-list mode, which would
// otherwise strip the unattended mcp__agent-relay__* access task routing needs.
func toolsValue(tools []string) string {
	out := slices.Clone(tools)
	if !slices.Contains(out, relayPermissionRule) {
		out = append(out, relayPermissionRule)
	}
	return strings.Join(out, ",")
}

// BuildLaunch assembles one agent's Claude launch command line. Fleet-global
// flags (e.g. --dangerously-skip-permissions) come first, then per-agent flags.
// The only tokens that reach the receiving shell are the binary path, global
// flags, allowlist-validated --model/--permission-mode values (no metachars by
// Validate), the persona FILE PATH (single-quoted), and the --allowedTools value
// (single-quoted as one argument). The multiline persona PROSE never appears
// here — only its file path does. personaPath is "" when the agent has none.
//
// Both launch sites (CreateSessions and the `fleet add` path) call this single
// builder so per-agent launch flags cannot drift between them.
func BuildLaunch(claudeBin string, globalFlags []string, a config.AgentConfig, personaPath string) string {
	cmd := claudeBin
	for _, f := range globalFlags {
		cmd += " " + f
	}
	if a.Model != "" {
		cmd += " --model " + a.Model
	}
	if a.PermissionMode != "" {
		cmd += " --permission-mode " + a.PermissionMode
	}
	if personaPath != "" {
		cmd += " --append-system-prompt-file " + shellSingleQuote(personaPath)
	}
	if len(a.Tools) > 0 {
		cmd += " --allowedTools " + shellSingleQuote(toolsValue(a.Tools))
	}
	return cmd
}
