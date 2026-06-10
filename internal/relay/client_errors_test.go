package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A JSON-RPC error envelope must surface as a Go error with the relay's message,
// not be silently swallowed into "empty/invalid relay response".
func TestCallSurfacesJSONRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom from relay"}}`)
	}))
	defer server.Close()

	err := NewClient(server.URL).Health()
	if err == nil {
		t.Fatal("expected JSON-RPC error to surface, got nil")
	}
	if !strings.Contains(err.Error(), "boom from relay") {
		t.Errorf("error should include the relay message, got: %v", err)
	}
}

// A non-2xx HTTP status must be an error, not parsed as a normal response.
func TestCallSurfacesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	err := NewClient(server.URL).Health()
	if err == nil {
		t.Fatal("expected HTTP 500 to surface as an error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention the HTTP status, got: %v", err)
	}
}

// A tool result flagged isError:true is a failure even though content is present.
func TestCallSurfacesToolIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"tool blew up"}],"isError":true}}`)
	}))
	defer server.Close()

	err := NewClient(server.URL).Health()
	if err == nil {
		t.Fatal("expected isError:true result to surface as an error, got nil")
	}
	if !strings.Contains(err.Error(), "tool blew up") {
		t.Errorf("error should include the tool error text, got: %v", err)
	}
}
