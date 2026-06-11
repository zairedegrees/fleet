// Package term holds helpers for safely rendering untrusted text to a terminal.
package term

import "strings"

// Sanitize strips terminal control characters from a string before it is
// printed. Relay-sourced values (agent names, roles, error text) are untrusted —
// any agent can register a name/role, and fleet may point at a shared relay — so
// a crafted value like "dev\x1b]0;…\x07" could otherwise hijack the operator's
// terminal title, write the clipboard (OSC 52), or move the cursor when rendered
// by `fleet status`, `fleet usage`, or the wizard. We drop C0 controls (incl.
// ESC, BEL, CR, LF), DEL, and C1 controls (incl. the 0x9b CSI byte); printable
// Unicode — accents, em-dash, emoji — is kept intact.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}
