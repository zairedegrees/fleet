package wizard

import "testing"

// deriveProjectName must produce a name that survives config.Validate(), so a
// folder like "site.com" or "My App" yields a safe project name instead of one
// that later fails launch (or worse, injects into the generated shell scripts).
func TestDeriveProjectName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/Users/x/site.com", "site-com"},
		{"/Users/x/My App", "My-App"},
		{"/home/u/clean-name", "clean-name"},
	}
	for _, tc := range tests {
		if got := deriveProjectName(tc.path); got != tc.want {
			t.Errorf("deriveProjectName(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
