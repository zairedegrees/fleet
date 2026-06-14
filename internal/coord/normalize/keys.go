// Package normalize snake_cases the keys of JSON payloads so coord persists
// message/metadata/result/value content with stable, consistent key naming. It
// is an independent implementation — the snake_case transformation is a standard
// one — not derived from any other relay's source.
package normalize

import (
	"encoding/json"
	"strings"
	"unicode"
)

// JSONKeys snake_cases every object key (at any depth) of a JSON object/array
// string. A value that is not a JSON object or array is returned byte-for-byte —
// opaque content such as a vault doc's markdown passes through untouched,
// surrounding whitespace included.
func JSONKeys(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return s
	}

	var decoded any
	if json.Unmarshal([]byte(trimmed), &decoded) != nil {
		return s
	}

	out, err := json.Marshal(normalizeTree(decoded))
	if err != nil {
		return s
	}
	return string(out)
}

// normalizeTree walks a decoded JSON value, snake-casing object keys at every
// depth. Scalars are returned unchanged; arrays are rewritten in place.
func normalizeTree(v any) any {
	switch node := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(node))
		for key, child := range node {
			result[snakeKey(key)] = normalizeTree(child)
		}
		return result
	case []any:
		for i := range node {
			node[i] = normalizeTree(node[i])
		}
		return node
	default:
		return v
	}
}

// snakeKey converts a camelCase / PascalCase key to snake_case. A run of capitals
// (an acronym) stays together, and an existing underscore is never doubled.
func snakeKey(key string) string {
	var b strings.Builder
	b.Grow(len(key) + 4)
	prevUpper, prevUnderscore := false, false
	for _, r := range key {
		switch {
		case unicode.IsUpper(r):
			if b.Len() > 0 && !prevUpper && !prevUnderscore {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			prevUpper, prevUnderscore = true, false
		default:
			b.WriteRune(r)
			prevUpper, prevUnderscore = false, r == '_'
		}
	}
	return b.String()
}
