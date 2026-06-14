package coord

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/relay"
)

func TestSetGetMemory(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "v1", "scope": "project"})

	var out struct {
		Count    int      `json:"count"`
		Memories []Memory `json:"memories"`
	}
	decodePayload(t, mustCall(t, s, "get_memory", map[string]any{"as": "a", "project": "p", "key": "k", "scope": "project"}), &out)
	if out.Count != 1 || out.Memories[0].Value != "v1" || out.Memories[0].Version != 1 {
		t.Fatalf("set/get: %+v", out)
	}
}

func TestMemoryUpsertVersioning(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "v1", "scope": "project"})

	var set struct {
		Memory Memory `json:"memory"`
	}
	decodePayload(t, mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "v2", "scope": "project"}), &set)
	if set.Memory.Version != 2 || set.Memory.Supersedes == nil {
		t.Fatalf("upsert did not version: %+v", set.Memory)
	}

	// get returns only the active v2 (old version archived).
	var out struct {
		Count    int      `json:"count"`
		Memories []Memory `json:"memories"`
	}
	decodePayload(t, mustCall(t, s, "get_memory", map[string]any{"as": "a", "project": "p", "key": "k", "scope": "project"}), &out)
	if out.Count != 1 || out.Memories[0].Value != "v2" || out.Memories[0].Version != 2 {
		t.Fatalf("get after upsert: %+v", out)
	}
}

func TestMemorySameValueTouchesOnly(t *testing.T) {
	s := New(newTestStore(t))
	var m1 struct {
		Memory Memory `json:"memory"`
	}
	decodePayload(t, mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "same", "scope": "project"}), &m1)

	var m2 struct {
		Memory Memory `json:"memory"`
	}
	decodePayload(t, mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "same", "scope": "project", "tags": []any{"t1"}}), &m2)
	if m2.Memory.ID != m1.Memory.ID || m2.Memory.Version != 1 {
		t.Errorf("same value should touch, not re-version: %+v", m2.Memory)
	}
	if m2.Memory.Tags != `["t1"]` {
		t.Errorf("tags not updated on touch: %q", m2.Memory.Tags)
	}
}

func TestMemoryConflictMode(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "v1", "scope": "project"})

	var set struct {
		Memory   Memory `json:"memory"`
		Conflict bool   `json:"conflict"`
	}
	decodePayload(t, mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "v2", "scope": "project", "upsert": false}), &set)
	if !set.Conflict || set.Memory.ConflictWith == nil {
		t.Fatalf("expected conflict flagged: %+v", set)
	}

	// Both versions stay active → get returns 2 and flags conflict.
	var out struct {
		Count    int  `json:"count"`
		Conflict bool `json:"conflict"`
	}
	decodePayload(t, mustCall(t, s, "get_memory", map[string]any{"as": "a", "project": "p", "key": "k", "scope": "project"}), &out)
	if out.Count != 2 || !out.Conflict {
		t.Errorf("conflict get: %+v", out)
	}
}

func TestMemoryScopeCascade(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "P", "scope": "project"})

	// No agent-scope value yet → cascade (agent→project→global) returns project.
	var out struct {
		Memories []Memory `json:"memories"`
	}
	decodePayload(t, mustCall(t, s, "get_memory", map[string]any{"as": "a", "project": "p", "key": "k"}), &out)
	if len(out.Memories) != 1 || out.Memories[0].Value != "P" {
		t.Fatalf("cascade should resolve to project: %+v", out)
	}

	// Add an agent-scope value → cascade now resolves to agent (higher precedence).
	mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "A", "scope": "agent"})
	var out2 struct {
		Memories []Memory `json:"memories"`
	}
	decodePayload(t, mustCall(t, s, "get_memory", map[string]any{"as": "a", "project": "p", "key": "k"}), &out2)
	if len(out2.Memories) != 1 || out2.Memories[0].Value != "A" {
		t.Fatalf("cascade should now resolve to agent: %+v", out2)
	}
}

func TestMemoryGlobalScopeAndCascadeFallthrough(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "g", "value": "G", "scope": "global"})

	// Explicit global scope.
	var out struct {
		Memories []Memory `json:"memories"`
	}
	decodePayload(t, mustCall(t, s, "get_memory", map[string]any{"as": "a", "project": "p", "key": "g", "scope": "global"}), &out)
	if len(out.Memories) != 1 || out.Memories[0].Value != "G" {
		t.Fatalf("global get: %+v", out)
	}

	// Cascade third hop: a different agent in a different project with no
	// agent/project value falls through to global.
	var out2 struct {
		Memories []Memory `json:"memories"`
	}
	decodePayload(t, mustCall(t, s, "get_memory", map[string]any{"as": "other", "project": "q", "key": "g"}), &out2)
	if len(out2.Memories) != 1 || out2.Memories[0].Value != "G" {
		t.Fatalf("cascade should fall through to global: %+v", out2)
	}
}

func TestSetMemoryInvalidScopeIsError(t *testing.T) {
	s := New(newTestStore(t))
	res := callTool(t, s, "set_memory", map[string]any{"as": "a", "project": "p", "key": "k", "value": "v", "scope": "weird"})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "invalid scope") {
		t.Errorf("invalid scope should error, got isErr=%v %q", res.IsError, res.Content[0].Text)
	}
}

// TestVaultRoundTripThroughClient drives fleet's PushVaultDoc (set_memory) with
// opaque markdown and asserts get_memory returns it byte-identical — the key
// normalization must not mutate non-JSON content.
func TestVaultRoundTripThroughClient(t *testing.T) {
	server := New(newTestStore(t))
	srv := httptest.NewServer(server.Handler())
	defer srv.Close()
	c := relay.NewClient(srv.URL + "/mcp")

	doc := "# Title\n\nSome **markdown** with a camelCaseWord and a bullet list.\n- one\n- two\n"
	if err := c.PushVaultDoc("proj", "design/notes.md", []byte(doc)); err != nil {
		t.Fatalf("PushVaultDoc: %v", err)
	}

	var out struct {
		Memories []Memory `json:"memories"`
	}
	decodePayload(t, mustCall(t, server, "get_memory", map[string]any{"project": "proj", "key": "vault:design/notes.md"}), &out)
	if len(out.Memories) != 1 {
		t.Fatalf("vault doc not retrievable: %+v", out)
	}
	if out.Memories[0].Value != doc {
		t.Errorf("vault round-trip mutated content:\n got %q\nwant %q", out.Memories[0].Value, doc)
	}
}
