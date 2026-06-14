package coordmgr

import (
	"os"
	"path/filepath"

	"github.com/zairedegrees/fleet/skill"
)

// skillDest is the install path (a var so tests can redirect it).
var skillDest = defaultSkillDest

func defaultSkillDest() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "skills", "relay", "SKILL.md")
}

// InstallSkill writes fleet's embedded MIT /relay skill (skill.Relay) so agents'
// `/relay` command resolves. It overwrites any prior copy (the skill is
// fleet-managed) so updates to the embedded version propagate. Unlike relaymgr's
// EnsureRelaySkill, this needs no network fetch.
func InstallSkill() error {
	dest := skillDest()
	if dest == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(skill.Relay), 0o644)
}
