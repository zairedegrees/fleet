package relay

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The relay's dispatch_task tool requires a `profile` (the agent's profile slug)
// and a P0–P3 priority. The old call used `assignee` + priority "high", which
// the relay rejects ("profile is required").
func TestDispatchTaskUsesProfileAndValidPriority(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"ok\":true}"}]}}`)
	}))
	defer server.Close()

	if err := NewClient(server.URL).DispatchTask("dev", "proj", "do the thing"); err != nil {
		t.Fatalf("DispatchTask failed: %v", err)
	}
	if !strings.Contains(gotBody, `"profile"`) || !strings.Contains(gotBody, `"dev"`) {
		t.Errorf("dispatch_task must target profile 'dev', got: %s", gotBody)
	}
	if strings.Contains(gotBody, `"assignee"`) {
		t.Errorf("dispatch_task must not use the rejected 'assignee' field, got: %s", gotBody)
	}
	if strings.Contains(gotBody, `"high"`) {
		t.Errorf("priority must be a P0–P3 enum, not 'high', got: %s", gotBody)
	}
}
