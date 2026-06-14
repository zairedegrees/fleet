package coord

import (
	"encoding/json"
	"testing"
)

// callTool marshals args and dispatches a tool through the registry, exactly as
// the HTTP layer would after decoding a tools/call request.
func callTool(t *testing.T, s *Server, name string, args map[string]any) toolResult {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return s.dispatch(name, raw)
}

// mustCall dispatches and fails if the tool returned an error result.
func mustCall(t *testing.T, s *Server, name string, args map[string]any) toolResult {
	t.Helper()
	res := callTool(t, s, name, args)
	if res.IsError {
		t.Fatalf("%s returned error: %s", name, res.Content[0].Text)
	}
	return res
}

// decodePayload unmarshals the double-encoded content[0].text payload into v.
func decodePayload(t *testing.T, res toolResult, v any) {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), v); err != nil {
		t.Fatalf("decode payload: %v (%s)", err, res.Content[0].Text)
	}
}
