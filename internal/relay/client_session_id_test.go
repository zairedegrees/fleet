package relay

import (
	"encoding/json"
	"testing"
)

// The coord serializes session_id on each agent; the client must decode it so
// `fleet cost` can map an agent to its transcript.
func TestAgentDecodesSessionID(t *testing.T) {
	raw := `{"name":"dev","status":"active","session_id":"sess-xyz"}`
	var a Agent
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatal(err)
	}
	if a.SessionID != "sess-xyz" {
		t.Errorf("session_id = %q, want sess-xyz", a.SessionID)
	}
}
