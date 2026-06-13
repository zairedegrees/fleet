package coord

import (
	"encoding/json"
	"fmt"
	"strings"
)

// handlerFunc implements one MCP tool against the server's store. args is the
// decoded tools/call "arguments" object.
type handlerFunc func(s *Server, args map[string]any) (toolResult, error)

// handlers is the tool registry. A bare tool name (no mcp__ prefix) maps to its
// implementation; anything absent degrades to a not-supported tool error rather
// than a transport failure, so agents calling out-of-scope wrai.th tools get a
// clean isError instead of a disconnect.
var handlers = map[string]handlerFunc{
	"list_orgs": handleListOrgs,
}

// dispatch routes a decoded tools/call to its handler and normalizes the result.
// A handler error becomes a tool-level error result (isError=true); an unknown
// tool becomes a not-supported error result. Neither is a protocol error.
func (s *Server) dispatch(name string, rawArgs json.RawMessage) toolResult {
	h, ok := handlers[name]
	if !ok {
		return resultError(fmt.Sprintf("tool %q is not supported by embedded coord", name))
	}
	res, err := h(s, parseArgs(rawArgs))
	if err != nil {
		return resultError(err.Error())
	}
	return res
}

func parseArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	return m
}

func argString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// resolveProject mirrors wrai.th: an empty/absent project means "default".
func resolveProject(m map[string]any) string {
	if p := argString(m, "project"); p != "" {
		return p
	}
	return "default"
}

// resolveAgent resolves the calling agent identity from the "as" argument,
// lowercased (names are case-folded on the wire); absent means "anonymous".
func resolveAgent(m map[string]any) string {
	if a := argString(m, "as"); a != "" {
		return strings.ToLower(a)
	}
	return "anonymous"
}
