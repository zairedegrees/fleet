package cost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanTranscriptSumsPerModelAndRespectsWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := strings.Join([]string{
		`{"type":"summary"}`, // no usage — skipped
		`{"type":"assistant","timestamp":"2026-06-29T10:00:00Z","message":{"model":"claude-opus-4-8","usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":50,"cache_creation_input_tokens":10}}}`,
		`{"type":"assistant","timestamp":"2026-06-28T10:00:00Z","message":{"model":"claude-opus-4-8","usage":{"input_tokens":999,"output_tokens":999}}}`, // before window — excluded
		`not json at all`, // malformed — skipped, not fatal
	}, "\n")
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	got, err := ScanTranscript(path, since)
	if err != nil {
		t.Fatal(err)
	}
	u := got["claude-opus-4-8"]
	if u.In != 100 || u.Out != 20 || u.CacheRead != 50 || u.CacheCreate != 10 {
		t.Errorf("window must exclude the 2026-06-28 turn; got %+v", u)
	}
}

func TestScanTranscriptZeroSinceCountsAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	os.WriteFile(path, []byte(
		`{"type":"assistant","timestamp":"2020-01-01T00:00:00Z","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":7}}}`+"\n"), 0o644)
	got, err := ScanTranscript(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got["claude-sonnet-4-6"].In != 7 {
		t.Errorf("zero since must include every turn; got %+v", got)
	}
}

func TestResolveTranscriptFindsNestedSession(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "-Users-x-proj")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	sid := "abc-123"
	want := filepath.Join(nested, sid+".jsonl")
	if err := os.WriteFile(want, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveTranscript(root, sid)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestResolveTranscriptMissingErrors(t *testing.T) {
	if _, err := ResolveTranscript(t.TempDir(), "nope"); err == nil {
		t.Error("missing transcript must error, not return an empty path")
	}
}
