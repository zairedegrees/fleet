package term

import "testing"

func TestSanitizeStripsControlSequences(t *testing.T) {
	c1 := string(rune(0x9b)) // C1 CSI, the single-byte form of ESC[
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"osc title set", "dev\x1b]0;pwned\x07", "dev]0;pwned"},
		{"osc52 clipboard", "ops\x1b]52;c;ZXZpbA==\x07", "ops]52;c;ZXZpbA=="},
		{"csi cursor", "quant\x1b[2J\x1b[H", "quant[2J[H"},
		{"bell", "a\x07b", "ab"},
		{"newline and cr", "line1\r\nline2", "line1line2"},
		{"c1 csi codepoint", "x" + c1 + "evil", "xevil"},
		{"del", "a\x7fb", "ab"},
		{"plain ascii untouched", "dev-ux", "dev-ux"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Sanitize(c.in); got != c.want {
				t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeKeepsAccentsAndEmoji(t *testing.T) {
	// French roles/names and emoji must survive — the strip targets control
	// characters only, not printable Unicode.
	for _, s := range []string{"Développeur", "auditeur—impitoyable", "R&D", "ops ✓", "café"} {
		if got := Sanitize(s); got != s {
			t.Errorf("Sanitize(%q) = %q, want unchanged", s, got)
		}
	}
}
