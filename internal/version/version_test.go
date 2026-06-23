package version

import "testing"

func TestVersionIsSet(t *testing.T) {
	if Version == "" {
		t.Fatal("version.Version must never be empty (source-build fallback)")
	}
}
