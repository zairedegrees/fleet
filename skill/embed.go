// Package skill embeds the in-repo Claude Code skills fleet ships, so they can
// be installed into ~/.claude/skills without a network fetch.
package skill

import _ "embed"

// Relay is fleet's MIT /relay skill: the agent coordination guide installed for
// the embedded coord backend.
//
//go:embed relay/SKILL.md
var Relay string
