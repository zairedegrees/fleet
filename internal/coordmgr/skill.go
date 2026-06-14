package coordmgr

import (
	_ "embed"
	"os"
	"path/filepath"
)

// relaySkill is fleet's own MIT /relay skill, embedded into the binary. Unlike
// relaymgr's EnsureRelaySkill (which fetched the AGPL skill from the wrai.th
// repo), this ships in-repo — so the embedded backend needs no network fetch.
//
//go:embed skill/relay/SKILL.md
var relaySkill string

// skillDest is the install path (a var so tests can redirect it).
var skillDest = defaultSkillDest

func defaultSkillDest() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "skills", "relay", "SKILL.md")
}

// InstallSkill writes the embedded /relay skill so agents' `/relay` command
// resolves. It overwrites any prior copy (the skill is fleet-managed) so updates
// to the embedded version propagate.
func InstallSkill() error {
	dest := skillDest()
	if dest == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(relaySkill), 0o644)
}
