package main

import (
	"strings"
	"testing"
)

func TestAgentLineSanitizesRelayName(t *testing.T) {
	// A ghost agent whose relay-sourced name carries an OSC title-set sequence
	// must not reach the terminal raw via `fleet status`.
	evil := "ghost" + string(rune(0x1b)) + "]0;pwned" + string(rune(0x07))
	line := agentLine(agentStatus{Agent: evil, RelayState: "inactive", Tasks: 0})
	if strings.ContainsRune(line, 0x1b) || strings.ContainsRune(line, 0x07) {
		t.Fatalf("agentLine leaked a control character: %q", line)
	}
	if !strings.Contains(line, "ghost]0;pwned") {
		t.Errorf("expected sanitized name in line, got %q", line)
	}
}
