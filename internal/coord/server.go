package coord

import (
	"encoding/json"
	"io"
	"net/http"
)

// handleMCP is the single JSON-RPC entry point. It speaks the subset of MCP the
// fleet client and the agent skill use: tools/call for every operation, plus a
// benign initialize handshake so a standard MCP client can connect. Tool-level
// problems are returned as a normal result with isError=true (HTTP 200) — only
// a malformed body or an unknown method uses the JSON-RPC error channel.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeRPCError(w, nil, rpcParseError, "read body: "+err.Error())
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeRPCError(w, nil, rpcParseError, "parse error: "+err.Error())
		return
	}

	switch req.Method {
	case "tools/call":
		var p toolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			s.writeRPCError(w, req.ID, rpcInvalidRequest, "invalid params: "+err.Error())
			return
		}
		s.writeResult(w, req.ID, s.dispatch(p.Name, p.Arguments))

	case "initialize":
		// A standard MCP client (the agents) opens with initialize. Advertise the
		// tools capability so it then calls tools/list to discover the catalog.
		s.writeRaw(w, req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "fleet-coord", "version": "0.1.10"},
		})

	case "tools/list":
		s.writeRaw(w, req.ID, map[string]any{"tools": advertisedTools()})

	case "ping":
		s.writeRaw(w, req.ID, map[string]any{})

	case "notifications/initialized", "notifications/cancelled":
		// JSON-RPC notifications carry no id and expect no result body; the MCP
		// streamable-HTTP transport answers them with 202 Accepted.
		w.WriteHeader(http.StatusAccepted)

	default:
		s.writeRPCError(w, req.ID, rpcMethodNotFound, "method not supported: "+req.Method)
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *Server) writeResult(w http.ResponseWriter, id json.RawMessage, res toolResult) {
	b, err := marshalResult(id, res)
	if err != nil {
		s.writeRPCError(w, id, rpcInvalidRequest, err.Error())
		return
	}
	s.writeJSON(w, b)
}

// writeRaw emits a non-tools/call success result (e.g. initialize) whose result
// object is the given value rather than a tool content envelope.
func (s *Server) writeRaw(w http.ResponseWriter, id json.RawMessage, result any) {
	b, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      idOrNull(id),
		"result":  result,
	})
	if err != nil {
		s.writeRPCError(w, id, rpcInvalidRequest, err.Error())
		return
	}
	s.writeJSON(w, b)
}

// writeRPCError emits a protocol-level JSON-RPC error. Per the wire contract
// these still carry HTTP 200 (the fleet client distinguishes tool vs protocol
// failures from the body, and reserves HTTP >= 400 for real transport faults).
func (s *Server) writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	b, err := marshalRPCError(id, code, msg)
	if err != nil {
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, b)
}
