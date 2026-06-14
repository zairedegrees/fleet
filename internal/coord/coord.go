// Package coord is fleet's native, embedded coordination core. It is an
// independent MIT reimplementation of the wrai.th relay's MCP-over-HTTP wire
// contract — same endpoint, same request/response shapes — written from the wire
// behavior, not its source. Backed by a pure-Go SQLite store
// (modernc.org/sqlite, no CGO), it lets fleet coordinate agents without
// downloading or running a separate relay binary.
package coord

import (
	"context"
	"net/http"
)

// Server serves the coordination API over HTTP. It owns the SQLite store and a
// single /mcp route; all tools are dispatched through that one endpoint.
type Server struct {
	store   *Store
	mux     *http.ServeMux
	httpSrv *http.Server
}

// New builds a Server backed by store. The store must already be open. The
// http.Server is constructed here (not in Serve) so Shutdown can never observe
// a nil server racing against a concurrent Serve — the detached `coord serve`
// child is exactly that Serve-in-goroutine / signal-calls-Shutdown pattern.
func New(store *Store) *Server {
	s := &Server{store: store, mux: http.NewServeMux()}
	s.mux.HandleFunc("/mcp", s.handleMCP)
	s.httpSrv = &http.Server{Handler: s.mux}
	return s
}

// Handler exposes the HTTP handler for in-process testing without binding a
// port.
func (s *Server) Handler() http.Handler { return s.mux }

// Serve binds addr and blocks serving the coordination API until Shutdown is
// called or the listener errors.
func (s *Server) Serve(addr string) error {
	s.httpSrv.Addr = addr
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server. It does not close the store.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}
