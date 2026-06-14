package coord

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/relay"
)

// TestFleetClientFullReplay (Tier-1) drives coord through EVERY method of fleet's
// own relay.Client and asserts no client guard fires (empty response, tool error,
// HTTP >= 400). This is the comprehensive consumer-contract gate: if coord's wire
// drifts from what the fleet binary expects, a method here returns an error.
func TestFleetClientFullReplay(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Handler())
	defer srv.Close()
	c := relay.NewClient(srv.URL + "/mcp")

	must := func(label string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: client guard fired: %v", label, err)
		}
	}

	must("Health", c.Health())
	must("EnsureProfile", c.EnsureProfile("ops", "monitor", "proj"))
	must("RegisterAgentFull", c.RegisterAgentFull(relay.AgentRegistration{
		Name: "ops", Project: "proj", Role: "monitor", ProfileSlug: "ops", ReportsTo: "", IsExecutive: false,
	}))
	must("RegisterAgent", c.RegisterAgent("worker", "proj", "builder", "worker"))

	agents, err := c.ListAgents("proj")
	must("ListAgents", err)
	if len(agents) != 2 {
		t.Fatalf("ListAgents returned %d agents, want 2", len(agents))
	}

	must("DispatchTask", c.DispatchTask("worker", "proj", "build the thing"))

	n, err := c.CountActiveTasks("proj", "worker")
	must("CountActiveTasks", err)
	if n != 1 {
		t.Errorf("CountActiveTasks = %d, want 1", n)
	}

	must("PushVaultDoc", c.PushVaultDoc("proj", "design/x.md", []byte("# Doc\n")))
	must("DeactivateAgent", c.DeactivateAgent("worker", "proj"))
}

// rawCall POSTs a verbatim JSON-RPC body and returns the decoded response plus
// the content[0].text payload string.
func rawCall(t *testing.T, h http.Handler, body string) (rpcResponse, string) {
	t.Helper()
	res, b := postMCP(t, h, body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", res.StatusCode, b)
	}
	var resp rpcResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal response: %v (%s)", err, b)
	}
	text := ""
	if resp.Result != nil && len(resp.Result.Content) > 0 {
		text = resp.Result.Content[0].Text
	}
	return resp, text
}

// TestGoldenRegisterPreserveRawWire (Tier-2) is the register-preserve gate at the
// raw-wire level: a re-register body that OMITS profile_slug (which the fleet
// client never does, but agents do) must not clobber it to NULL.
func TestGoldenRegisterPreserveRawWire(t *testing.T) {
	h := New(newTestStore(t)).Handler()

	// Exact shape client.go RegisterAgentFull sends (all fields present).
	resp1, _ := rawCall(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"register_agent","arguments":{"name":"ops","project":"p","role":"monitor","profile_slug":"ops-slug","reports_to":"","is_executive":false}}}`)
	if resp1.Error != nil || resp1.Result.IsError {
		t.Fatalf("initial register errored: %+v", resp1)
	}

	// Bare respawn omitting profile_slug.
	rawCall(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"register_agent","arguments":{"name":"ops","project":"p","role":"monitor-v2"}}}`)

	_, text := rawCall(t, h, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_agents","arguments":{"project":"p"}}}`)
	var out struct {
		Agents []struct {
			Name        string `json:"name"`
			ProfileSlug string `json:"profile_slug"`
		} `json:"agents"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("decode list_agents payload: %v (%s)", err, text)
	}
	if len(out.Agents) != 1 || out.Agents[0].ProfileSlug != "ops-slug" {
		t.Fatalf("raw-wire re-register clobbered profile_slug: %+v", out.Agents)
	}
}

func TestGoldenDoubleEncodedEnvelope(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	_, b := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_orgs","arguments":{}}}`)
	// content[0].text must be an ESCAPED JSON string, not a nested object.
	if !strings.Contains(string(b), `"text":"{`) {
		t.Errorf("envelope not double-encoded: %s", b)
	}
}

func TestGoldenListTasksCountAfterLimit(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	for i := 0; i < 3; i++ {
		rawCall(t, h, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"dispatch_task","arguments":{"project":"p","profile":"w","title":"t%d"}}}`, i))
	}
	_, text := rawCall(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_tasks","arguments":{"project":"p","profile":"w","status":"active","limit":2}}}`)
	var out struct {
		Count int               `json:"count"`
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || len(out.Tasks) != 2 {
		t.Errorf("count-after-limit: count=%d tasks=%d, want 2/2", out.Count, len(out.Tasks))
	}
}

func TestGoldenListTasksTruncation(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	long := strings.Repeat("a", 250)
	rawCall(t, h, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"dispatch_task","arguments":{"project":"p","profile":"w","title":"x","description":"%s"}}}`, long))
	_, text := rawCall(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_tasks","arguments":{"project":"p","profile":"w"}}}`)
	var out struct {
		Tasks []struct {
			Description string `json:"description"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if d := out.Tasks[0].Description; !strings.HasSuffix(d, "…") || len([]rune(d)) != 201 {
		t.Errorf("list-view description not truncated to 200+ellipsis: %d runes", len([]rune(d)))
	}
}

func TestGoldenToolErrorIsNotProtocolError(t *testing.T) {
	h := New(newTestStore(t)).Handler()
	// dispatch_task missing profile → tool-level error (isError, HTTP 200), exact
	// message, NOT a JSON-RPC protocol error.
	resp, text := rawCall(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"dispatch_task","arguments":{"project":"p","title":"x"}}}`)
	if resp.Error != nil {
		t.Fatalf("got protocol error, want tool-level error: %+v", resp.Error)
	}
	if resp.Result == nil || !resp.Result.IsError || text != "profile is required" {
		t.Errorf("tool error wrong: isErr=%v text=%q", resp.Result != nil && resp.Result.IsError, text)
	}
}
