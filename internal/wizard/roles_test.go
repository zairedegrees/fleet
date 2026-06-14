package wizard

import "testing"

func TestDefaultModelForRole(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		// Deep / adversarial / open-ended judgement → Opus.
		{"architect", "opus"},
		{"auditor", "opus"},
		{"researcher", "opus"},
		{"quant", "opus"},
		{"security", "opus"},
		// Bounded implementation → Sonnet.
		{"dev", "sonnet"},
		{"frontend", "sonnet"},
		{"ux-designer", "sonnet"},
		{"ops", "sonnet"},
		{"docs", "sonnet"},
		// Cheap watchers → Haiku.
		{"notifier", "haiku"},
		// Unknown role falls back to the safe builder default.
		{"whatever-new-role", "sonnet"},
	}
	for _, tt := range tests {
		if got := defaultModelForRole(tt.role); got != tt.want {
			t.Errorf("defaultModelForRole(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}
