package runner

import (
	"testing"
)

func TestSessionName(t *testing.T) {
	name := SessionName("quant")
	if name != "pm-quant" {
		t.Errorf("got %q, want %q", name, "pm-quant")
	}
}

func TestBuildCreateCmd(t *testing.T) {
	args := buildCreateArgs("pm-test", "/tmp/project")
	expected := []string{"new-session", "-d", "-s", "pm-test", "-c", "/tmp/project"}
	if len(args) != len(expected) {
		t.Fatalf("got %d args, want %d", len(args), len(expected))
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg %d: got %q, want %q", i, arg, expected[i])
		}
	}
}

func TestBuildSendKeysCmd(t *testing.T) {
	args := buildSendKeysArgs("pm-test", "hello world")
	expected := []string{"send-keys", "-t", "pm-test", "hello world", "Enter"}
	if len(args) != len(expected) {
		t.Fatalf("got %d args, want %d", len(args), len(expected))
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg %d: got %q, want %q", i, arg, expected[i])
		}
	}
}
