package config

const (
	PostureIdle    = "idle"
	PostureBounded = "bounded"
	PostureAlways  = "always"
)

var validPostures = map[string]bool{
	PostureIdle: true, PostureBounded: true, PostureAlways: true,
}

// EffectivePosture returns the agent's posture, falling back to the legacy
// auto_talk boolean when posture is unset (pre-Normalize configs).
func (a AgentConfig) EffectivePosture() string {
	if a.Posture != "" {
		return a.Posture
	}
	if a.AutoTalk {
		return PostureAlways
	}
	return PostureIdle
}

func (a AgentConfig) IsBounded() bool    { return a.EffectivePosture() == PostureBounded }
func (a AgentConfig) GreetsAtBoot() bool { return a.EffectivePosture() == PostureAlways }

// Normalize canonicalizes every agent's posture: it fills Posture from the
// legacy auto_talk boolean when unset, and keeps auto_talk as a derived mirror
// of "greets at boot" (posture == always) so older tooling/tests that read
// auto_talk still see a coherent value. Idempotent.
func (cfg *FleetConfig) Normalize() {
	for i := range cfg.Agents {
		p := cfg.Agents[i].EffectivePosture()
		cfg.Agents[i].Posture = p
		cfg.Agents[i].AutoTalk = p == PostureAlways
	}
}
