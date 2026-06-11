package relaymgr

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const defaultSkillRawURL = "https://raw.githubusercontent.com/" + releaseRepo + "/main/skill/relay.md"

var skillRawURL = defaultSkillRawURL

// EnsureRelaySkill installs wrai.th's /relay skill into ~/.claude/skills/relay so
// the agents' `/relay talk` wake command resolves. Idempotent. Caller obtains
// consent on first use (shared with the binary download).
func EnsureRelaySkill() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return ensureRelaySkillAt(filepath.Join(home, ".claude", "skills", "relay", "SKILL.md"))
}

func ensureRelaySkillAt(dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil // already installed
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	resp, err := http.Get(skillRawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("fetch relay skill: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0644)
}
