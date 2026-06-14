package coord

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhoamiSaltValidation(t *testing.T) {
	s := New(newTestStore(t))
	if r := callTool(t, s, "whoami", map[string]any{}); !r.IsError || !strings.Contains(r.Content[0].Text, "salt is required") {
		t.Errorf("missing salt: isErr=%v %q", r.IsError, r.Content[0].Text)
	}
	if r := callTool(t, s, "whoami", map[string]any{"salt": "abc"}); !r.IsError || !strings.Contains(r.Content[0].Text, "too short") {
		t.Errorf("short salt: isErr=%v %q", r.IsError, r.Content[0].Text)
	}
}

func TestWhoamiFindsSessionFromTranscript(t *testing.T) {
	dir := t.TempDir()
	sid := "11111111-2222-4333-8444-555555555555"
	if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"),
		[]byte(`{"role":"user","content":"my salt is purple-otter-9281"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := claudeProjectsDir
	claudeProjectsDir = func() string { return dir }
	defer func() { claudeProjectsDir = old }()

	s := New(newTestStore(t))
	var out struct {
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
	}
	decodePayload(t, mustCall(t, s, "whoami", map[string]any{"salt": "purple-otter-9281"}), &out)
	if out.SessionID != sid {
		t.Errorf("session_id = %q, want %q", out.SessionID, sid)
	}
	if !strings.HasSuffix(out.TranscriptPath, sid+".jsonl") {
		t.Errorf("transcript_path = %q", out.TranscriptPath)
	}
}

func TestWhoamiSaltNotFound(t *testing.T) {
	dir := t.TempDir()
	old := claudeProjectsDir
	claudeProjectsDir = func() string { return dir }
	defer func() { claudeProjectsDir = old }()

	s := New(newTestStore(t))
	r := callTool(t, s, "whoami", map[string]any{"salt": "absent-salt-words"})
	if !r.IsError || !strings.Contains(r.Content[0].Text, "not found") {
		t.Errorf("expected not-found error: isErr=%v %q", r.IsError, r.Content[0].Text)
	}
}

func TestWhoamiScansTailWindowOnly(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("filler line padding xxxxxxxxxxxxxxxxxxxxxxxxxxxx\n", 3000) // > 64KB

	// File 1: salt only in the final line (inside the last-64KB window) -> found.
	sid1 := "aaaaaaaa-0000-4000-8000-000000000001"
	if err := os.WriteFile(filepath.Join(dir, sid1+".jsonl"), []byte(big+`{"c":"tail zzz-zebra-7777"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// File 2: salt only in the FIRST line, then > 64KB of filler -> before the
	// window -> NOT found.
	sid2 := "bbbbbbbb-0000-4000-8000-000000000002"
	if err := os.WriteFile(filepath.Join(dir, sid2+".jsonl"), []byte(`{"c":"early before-window-9999"}`+"\n"+big), 0o644); err != nil {
		t.Fatal(err)
	}

	old := claudeProjectsDir
	claudeProjectsDir = func() string { return dir }
	defer func() { claudeProjectsDir = old }()

	s := New(newTestStore(t))
	var found struct {
		SessionID string `json:"session_id"`
	}
	decodePayload(t, mustCall(t, s, "whoami", map[string]any{"salt": "zzz-zebra-7777"}), &found)
	if found.SessionID != sid1 {
		t.Errorf("tail-window salt resolved to %q, want %q", found.SessionID, sid1)
	}

	r := callTool(t, s, "whoami", map[string]any{"salt": "before-window-9999"})
	if !r.IsError || !strings.Contains(r.Content[0].Text, "not found") {
		t.Errorf("pre-window salt should be not found, got isErr=%v %q", r.IsError, r.Content[0].Text)
	}
}

func TestGetSessionContextDispatchedAndNoProfile(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_agent", map[string]any{"name": "w1", "project": "p"}) // no profile_slug
	mustCall(t, s, "dispatch_task", map[string]any{"as": "w1", "project": "p", "profile": "other", "title": "do"})

	var out struct {
		Profile      *Profile `json:"profile"`
		PendingTasks struct {
			DispatchedByMe []Task `json:"dispatched_by_me"`
		} `json:"pending_tasks"`
	}
	decodePayload(t, mustCall(t, s, "get_session_context", map[string]any{"as": "w1", "project": "p"}), &out)
	if out.Profile != nil {
		t.Errorf("agent without profile_slug should have nil profile, got %+v", out.Profile)
	}
	if len(out.PendingTasks.DispatchedByMe) != 1 {
		t.Errorf("dispatched_by_me = %d, want 1", len(out.PendingTasks.DispatchedByMe))
	}
}

func TestGetSessionContextBundles(t *testing.T) {
	s := New(newTestStore(t))
	mustCall(t, s, "register_profile", map[string]any{"slug": "worker", "name": "Worker", "project": "p"})
	mustCall(t, s, "register_agent", map[string]any{"name": "w1", "project": "p", "profile_slug": "worker"})
	mustCall(t, s, "dispatch_task", map[string]any{"project": "p", "profile": "worker", "title": "do it"})
	mustCall(t, s, "send_message", map[string]any{"as": "boss", "to": "w1", "project": "p", "content": "hi"})
	mustCall(t, s, "set_memory", map[string]any{"as": "w1", "project": "p", "key": "k", "value": "v", "scope": "project"})

	var out struct {
		Agent        string   `json:"agent"`
		Project      string   `json:"project"`
		Profile      *Profile `json:"profile"`
		PendingTasks struct {
			AssignedToMe   []Task `json:"assigned_to_me"`
			DispatchedByMe []Task `json:"dispatched_by_me"`
		} `json:"pending_tasks"`
		UnreadMessages   []map[string]any `json:"unread_messages"`
		RelevantMemories []Memory         `json:"relevant_memories"`
	}
	decodePayload(t, mustCall(t, s, "get_session_context", map[string]any{"as": "w1", "project": "p"}), &out)

	if out.Agent != "w1" || out.Project != "p" {
		t.Errorf("agent/project: %q/%q", out.Agent, out.Project)
	}
	// Profile is auto-detected from the registered agent.
	if out.Profile == nil || out.Profile.Slug != "worker" {
		t.Errorf("profile auto-detect: %+v", out.Profile)
	}
	// The unclaimed pending task for w1's profile shows as assigned-to-me.
	if len(out.PendingTasks.AssignedToMe) != 1 {
		t.Errorf("assigned_to_me = %d, want 1", len(out.PendingTasks.AssignedToMe))
	}
	// Both the boss message and the dispatch auto-notify are unread.
	if len(out.UnreadMessages) != 2 {
		t.Errorf("unread_messages = %d, want 2", len(out.UnreadMessages))
	}
	if len(out.RelevantMemories) != 1 {
		t.Errorf("relevant_memories = %d, want 1", len(out.RelevantMemories))
	}
}
