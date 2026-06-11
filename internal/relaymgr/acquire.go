package relaymgr

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const releaseRepo = "Synergix-lab/WRAI.TH"

// fetch is the acquisition seam: obtain the agent-relay binary into dest.
var fetch = defaultFetch

// runCmd is the build seam.
var runCmd = exec.Command

// EnsureBinary resolves the managed binary, fetching it (download then build
// fallback) on first use. Caller is responsible for obtaining consent first.
func EnsureBinary() (string, error) { return ensureBinaryAt(BinPath()) }

func ensureBinaryAt(bin string) (string, error) {
	if fi, err := os.Stat(bin); err == nil && !fi.IsDir() {
		return bin, nil
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0755); err != nil {
		return "", err
	}
	if err := fetch(bin); err != nil {
		return "", err
	}
	return bin, nil
}

// defaultFetch tries the prebuilt release, then build-from-source.
func defaultFetch(dest string) error {
	url, err := latestAssetURL()
	if err == nil {
		if derr := downloadPrebuilt(url, dest); derr == nil {
			return nil
		} else {
			err = derr
		}
	}
	if berr := buildFromSource(dest); berr == nil {
		return nil
	} else {
		return fmt.Errorf("could not obtain agent-relay automatically (download: %v; build: %v) — install wrai.th manually or pass --relay-url", err, berr)
	}
}

// latestAssetURL resolves the release asset for this OS/arch.
func latestAssetURL() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/" + releaseRepo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("github releases/latest: status %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	asset := fmt.Sprintf("agent-relay-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", releaseRepo, rel.TagName, asset), nil
}

// downloadPrebuilt GETs a .tar.gz, extracts the agent-relay entry to dest 0755.
func downloadPrebuilt(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("archive %s has no agent-relay entry", url)
		}
		if err != nil {
			return err
		}
		if filepath.Base(h.Name) == "agent-relay" {
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
			return os.Chmod(dest, 0755)
		}
	}
}

// buildFromSource clones wrai.th and builds, falling back when no prebuilt fits.
func buildFromSource(dest string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("no Go toolchain for source build: %w", err)
	}
	tmp, err := os.MkdirTemp("", "wraith-src-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if out, err := runCmd("git", "clone", "--depth", "1", "https://github.com/"+releaseRepo+".git", tmp).CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %v: %s", err, out)
	}
	build := runCmd("go", "build", "-tags", "fts5", "-o", dest, ".")
	build.Dir = tmp
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("go build (needs a C compiler for SQLite): %v: %s", err, out)
	}
	return os.Chmod(dest, 0755)
}
