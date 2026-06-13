package normalize

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONKeysSnakeCases(t *testing.T) {
	in := `{"taskId":"abc","nestedObj":{"camelCase":1},"arr":[{"fooBar":2}]}`
	got := JSONKeys(in)

	// Keys are snake_cased at every depth; values are untouched.
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("output not valid JSON: %v (%s)", err, got)
	}
	for _, want := range []string{`"task_id"`, `"nested_obj"`, `"camel_case"`, `"foo_bar"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing snake_cased key %s in %s", want, got)
		}
	}
	if m["task_id"] != "abc" {
		t.Errorf("value mutated: %v", m["task_id"])
	}
}

func TestJSONKeysNonJSONUnchanged(t *testing.T) {
	// Opaque (non-JSON-object) values must round-trip byte-identical — this is
	// what keeps a vault doc transported via set_memory from being corrupted.
	for _, in := range []string{
		"hello world",
		"New task: ship the thing",
		"# Heading\n\nSome **markdown** with a camelCase word.",
		"",
	} {
		if got := JSONKeys(in); got != in {
			t.Errorf("JSONKeys(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestJSONKeysAlreadySnakeIsStable(t *testing.T) {
	in := `{"task_id":"x","sub":{"already_snake":true}}`
	if got := JSONKeys(in); !strings.Contains(got, `"task_id"`) || !strings.Contains(got, `"already_snake"`) {
		t.Errorf("snake keys not preserved: %s", got)
	}
}
