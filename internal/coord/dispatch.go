package coord

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	"list_orgs":        handleListOrgs,
	"register_agent":   handleRegisterAgent,
	"list_agents":      handleListAgents,
	"deactivate_agent": handleDeactivateAgent,
	"register_profile": handleRegisterProfile,
	"dispatch_task":    handleDispatchTask,
	"list_tasks":       handleListTasks,
	"claim_task":       handleClaimTask,
	"start_task":       handleStartTask,
	"complete_task":    handleCompleteTask,
	"block_task":       handleBlockTask,
	"get_task":         handleGetTask,
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

// argStringDefault returns the string arg, or def when absent/empty — mirroring
// wrai.th's req.GetString(key, default).
func argStringDefault(m map[string]any, key, def string) string {
	if v := argString(m, key); v != "" {
		return v
	}
	return def
}

// argBool mirrors mcp-go's GetBool coercion: a JSON bool, a string ("true"),
// or a number all resolve. Matching the coercion matters because presence is
// detected separately (argPresent) — a stringly-typed "true" must read as true,
// not fall to the default and clobber a preserved field on respawn.
func argBool(m map[string]any, key string, def bool) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	case float64:
		return v != 0
	}
	return def
}

// argInt mirrors mcp-go's GetInt coercion: JSON numbers decode as float64, but
// a string ("500") or a native int also resolve.
func argInt(m map[string]any, key string, def int) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// argJSONArray ports wrai.th's normalizeJSONArrayParam: a JSON string is kept
// as-is, a non-JSON string is wrapped as a single-element array, a native
// array/object is re-marshaled, and an absent/empty value yields "[]".
func argJSONArray(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		if s == "" {
			return "[]"
		}
		if json.Valid([]byte(s)) {
			return s
		}
		b, _ := json.Marshal([]string{s})
		return string(b)
	}
	if raw, ok := m[key]; ok && raw != nil {
		if b, err := json.Marshal(raw); err == nil {
			return string(b)
		}
	}
	return "[]"
}

// argPresent reports whether key was provided at all — the distinction the
// register_agent preserve-on-omit logic hinges on (an omitted optional field
// must be preserved, not cleared).
func argPresent(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

// optionalString returns nil for an empty string, else a pointer to it.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optionalStringLower lowercases then wraps; used for case-folded fields like
// reports_to.
func optionalStringLower(s string) *string {
	if s == "" {
		return nil
	}
	l := strings.ToLower(s)
	return &l
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
