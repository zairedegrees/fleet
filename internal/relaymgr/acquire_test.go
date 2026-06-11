package relaymgr

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// tarGzWith builds an in-memory .tar.gz containing one file named "agent-relay".
func tarGzWith(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "agent-relay", Mode: 0755, Size: int64(len(content))})
	tw.Write([]byte(content))
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestDownloadPrebuiltWritesExecutable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarGzWith(t, "#!/bin/true\n"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "agent-relay")
	if err := downloadPrebuilt(srv.URL, dest); err != nil {
		t.Fatalf("downloadPrebuilt: %v", err)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm()&0100 == 0 {
		t.Error("downloaded binary must be executable")
	}
}

func TestEnsureBinaryReturnsExistingWithoutFetch(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent-relay")
	os.WriteFile(bin, []byte("x"), 0755)

	fetched := false
	fetch = func(dest string) error { fetched = true; return nil } // seam
	defer func() { fetch = defaultFetch }()

	got, err := ensureBinaryAt(bin)
	if err != nil || got != bin {
		t.Fatalf("ensureBinaryAt = %q, %v", got, err)
	}
	if fetched {
		t.Error("should not fetch when binary already present")
	}
}

func TestEnsureBinaryFetchesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent-relay")
	fetch = func(dest string) error { return os.WriteFile(dest, []byte("x"), 0755) }
	defer func() { fetch = defaultFetch }()

	got, err := ensureBinaryAt(bin)
	if err != nil || got != bin {
		t.Fatalf("ensureBinaryAt = %q, %v", got, err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Error("binary should have been fetched")
	}
}
