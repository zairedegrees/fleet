package coord

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postMCP(t *testing.T, h http.Handler, body string) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func TestBareToolsCallNoHandshake(t *testing.T) {
	h := New(newTestStore(t)).Handler()

	// An unknown tool, with no prior initialize, must still get HTTP 200 and a
	// tool-level error result — never a transport failure or a 4xx/5xx.
	res, b := postMCP(t, h,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"definitely_not_a_tool","arguments":{}}}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var resp rpcResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, b)
	}
	if resp.Error != nil {
		t.Fatalf("got protocol error, want tool-level error: %v", resp.Error)
	}
	if resp.Result == nil || !resp.Result.IsError {
		t.Fatalf("expected isError result, got %s", b)
	}
}

func TestMalformedBodyIsJSONRPCError(t *testing.T) {
	h := New(newTestStore(t)).Handler()

	res, b := postMCP(t, h, `{this is not json`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var resp rpcResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, b)
	}
	if resp.Error == nil {
		t.Fatalf("expected JSON-RPC error, got %s", b)
	}
	if resp.Error.Code != rpcParseError {
		t.Errorf("error code = %d, want %d", resp.Error.Code, rpcParseError)
	}
}

func TestInitializeIsBenign(t *testing.T) {
	h := New(newTestStore(t)).Handler()

	res, b := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var resp struct {
		Error  *rpcError      `json:"error"`
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, b)
	}
	if resp.Error != nil {
		t.Fatalf("initialize returned error: %v", resp.Error)
	}
	if resp.Result["protocolVersion"] == nil {
		t.Errorf("initialize result missing protocolVersion: %s", b)
	}
}

func TestUnknownMethodIsMethodNotFound(t *testing.T) {
	h := New(newTestStore(t)).Handler()

	// An unknown METHOD (vs an unknown tool) uses the JSON-RPC error channel, not
	// the tool-error result — these two paths must not be confused.
	res, b := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var resp rpcResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, b)
	}
	if resp.Error == nil {
		t.Fatalf("expected JSON-RPC error, got %s", b)
	}
	if resp.Error.Code != rpcMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, rpcMethodNotFound)
	}
}

func TestToolsCallMalformedParamsIsInvalidRequest(t *testing.T) {
	h := New(newTestStore(t)).Handler()

	// params is a JSON array, not the {name,arguments} object — a protocol error.
	res, b := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":[1,2,3]}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var resp rpcResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, b)
	}
	if resp.Error == nil {
		t.Fatalf("expected JSON-RPC error, got %s", b)
	}
	if resp.Error.Code != rpcInvalidRequest {
		t.Errorf("error code = %d, want %d", resp.Error.Code, rpcInvalidRequest)
	}
}
