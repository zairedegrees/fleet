package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasStatusLine(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), 0644)
		return p
	}
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"present", write("a.json", `{"statusLine":{"type":"command","command":"x"}}`), true},
		{"absent key", write("b.json", `{"model":"opus"}`), false},
		{"empty value", write("c.json", `{"statusLine":{}}`), false},
		{"missing file", filepath.Join(dir, "nope.json"), false},
		{"malformed", write("d.json", `{not json`), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasStatusLine(c.path); got != c.want {
				t.Errorf("hasStatusLine(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestMergeStatusLineFreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	if err := mergeStatusLine(path, "node /x/index.mjs"); err != nil {
		t.Fatalf("mergeStatusLine: %v", err)
	}
	if !hasStatusLine(path) {
		t.Error("expected statusLine after merge")
	}
}

func TestMergeStatusLinePreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	os.WriteFile(path, []byte(`{"theme":"dark"}`), 0644)
	if err := mergeStatusLine(path, "node /x/index.mjs"); err != nil {
		t.Fatalf("mergeStatusLine: %v", err)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	if !strings.Contains(s, "theme") || !strings.Contains(s, "statusLine") {
		t.Errorf("merge must keep theme and add statusLine, got %s", s)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Error("expected a .bak backup")
	}
}

func TestMergeStatusLineMalformedRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	os.WriteFile(path, []byte(`{bad`), 0644)
	if err := mergeStatusLine(path, "node /x/index.mjs"); err == nil {
		t.Error("expected error on malformed existing file")
	}
	b, _ := os.ReadFile(path)
	if string(b) != `{bad` {
		t.Error("malformed file must not be clobbered")
	}
}
