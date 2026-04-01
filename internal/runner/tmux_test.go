package runner

import (
	"testing"
)

func TestSessionName(t *testing.T) {
	name := SessionName("demo-app", "quant")
	if name != "fleet-demo-app-quant" {
		t.Errorf("got %q, want %q", name, "fleet-demo-app-quant")
	}
}

func TestSessionNameSanitizesDots(t *testing.T) {
	name := SessionName("my.project.v2", "dev")
	if name != "fleet-my-project-v2-dev" {
		t.Errorf("got %q, want %q", name, "fleet-my-project-v2-dev")
	}
}

func TestBuildCreateCmd(t *testing.T) {
	args := buildCreateArgs("fleet-myproject-test", "/tmp/project")
	expected := []string{"new-session", "-d", "-s", "fleet-myproject-test", "-c", "/tmp/project"}
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
	args := buildSendKeysArgs("fleet-myproject-test", "hello world")
	expected := []string{"send-keys", "-t", "fleet-myproject-test", "hello world", "Enter"}
	if len(args) != len(expected) {
		t.Fatalf("got %d args, want %d", len(args), len(expected))
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg %d: got %q, want %q", i, arg, expected[i])
		}
	}
}

func TestAgentFromSession(t *testing.T) {
	agent := AgentFromSession("demo-app", "fleet-demo-app-brain-dev")
	if agent != "brain-dev" {
		t.Errorf("got %q, want %q", agent, "brain-dev")
	}
}

func TestSanitizeProject(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"has.dots", "has-dots"},
		{"a.b.c", "a-b-c"},
		{"no-dots", "no-dots"},
	}
	for _, tc := range tests {
		got := sanitizeProject(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeProject(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDetectConflicts(t *testing.T) {
	// DetectConflicts checks HasSession which talks to tmux — in a test
	// environment without tmux running, no sessions exist, so no conflicts.
	conflicts := DetectConflicts("testproj", []string{"a", "b"})
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts without tmux, got %v", conflicts)
	}
}
