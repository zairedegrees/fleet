package runner

import "testing"

// WakeAgent / dispatch must submit /relay talk with a SEPARATE Enter — the
// combined "text Enter" send-keys is swallowed by the skill autocomplete and the
// agent never wakes. buildTypeArgs types without submitting.
func TestBuildTypeArgsDoesNotSubmit(t *testing.T) {
	args := buildTypeArgs("fleet-p-dev", "/relay talk")
	for _, a := range args {
		if a == "Enter" {
			t.Errorf("type args must not contain Enter (would submit prematurely): %v", args)
		}
	}
	want := []string{"send-keys", "-t", "fleet-p-dev", "/relay talk"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, args[i], want[i])
		}
	}
}

func TestBuildEnterArgs(t *testing.T) {
	args := buildEnterArgs("fleet-p-dev")
	want := []string{"send-keys", "-t", "fleet-p-dev", "Enter"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, args[i], want[i])
		}
	}
}
