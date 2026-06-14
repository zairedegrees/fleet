// Package normalize ports wrai.th's JSON key normalization so coord persists
// message/metadata/result/value payloads with the same snake_cased keys the
// relay produces, keeping stored data wire-compatible.
package normalize

import (
	"encoding/json"
	"strings"
	"unicode"
)

// JSONKeys normalizes all keys in a JSON object/array string from camelCase to
// snake_case. If the input is not a JSON object or array, the ORIGINAL string is
// returned byte-for-byte — opaque values (e.g. a vault doc's raw markdown) pass
// through untouched, including any surrounding whitespace. (This is a deliberate
// improvement over wrai.th, whose TrimSpace clips opaque values; JSON values
// normalize identically.)
func JSONKeys(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return s
	}

	var raw any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return s
	}

	out, err := json.Marshal(convertKeys(raw))
	if err != nil {
		return s
	}
	return string(out)
}

func convertKeys(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[toSnakeCase(k)] = convertKeys(vv)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = convertKeys(vv)
		}
		return out
	default:
		return v
	}
}

// toSnakeCase converts camelCase/PascalCase to snake_case; already-snake_case
// strings pass through unchanged.
func toSnakeCase(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rune(s[i-1])
				if prev != '_' && !unicode.IsUpper(prev) {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
