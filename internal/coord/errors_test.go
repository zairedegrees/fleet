package coord

import "testing"

// TestRequiredArgErrors locks the required-field error paths: each must return a
// tool-level error result (isError=true) with the exact message, exercising the
// error envelope the fleet client guards on.
func TestRequiredArgErrors(t *testing.T) {
	s := New(newTestStore(t))
	cases := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"dispatch_task", map[string]any{"project": "p", "title": "t"}, "profile is required"},
		{"dispatch_task", map[string]any{"project": "p", "profile": "x"}, "title is required"},
		{"register_agent", map[string]any{"project": "p"}, "name is required"},
		{"register_profile", map[string]any{"project": "p"}, "slug is required"},
		{"register_profile", map[string]any{"project": "p", "slug": "s"}, "name is required"},
		{"deactivate_agent", map[string]any{"project": "p"}, "name is required"},
	}
	for _, c := range cases {
		res := callTool(t, s, c.tool, c.args)
		if !res.IsError {
			t.Errorf("%s%v: expected error result, got success", c.tool, c.args)
			continue
		}
		if res.Content[0].Text != c.want {
			t.Errorf("%s%v: error = %q, want %q", c.tool, c.args, res.Content[0].Text, c.want)
		}
	}
}
