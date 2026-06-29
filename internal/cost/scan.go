package cost

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// transcriptLine is the subset of a Claude Code transcript JSONL line we read.
type transcriptLine struct {
	Timestamp string `json:"timestamp"`
	Message   struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens         int64 `json:"input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ScanTranscript sums message.usage per model from the transcript at path,
// counting only turns whose timestamp is at or after since. A zero since means
// no lower bound. A line that fails to parse is skipped, not fatal.
func ScanTranscript(path string, since time.Time) (map[string]Usage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]Usage{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024) // transcript lines can be large
	for sc.Scan() {
		var line transcriptLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}
		if line.Message.Usage == nil {
			continue
		}
		if !since.IsZero() {
			ts, err := time.Parse(time.RFC3339, line.Timestamp)
			if err != nil || ts.Before(since) {
				continue
			}
		}
		u := out[line.Message.Model]
		u.In += line.Message.Usage.InputTokens
		u.Out += line.Message.Usage.OutputTokens
		u.CacheRead += line.Message.Usage.CacheReadTokens
		u.CacheCreate += line.Message.Usage.CacheCreationTokens
		out[line.Message.Model] = u
	}
	return out, sc.Err()
}

// DefaultProjectsDir is where Claude Code stores per-project transcripts.
func DefaultProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// ResolveTranscript finds the <sessionID>.jsonl transcript anywhere under
// projectsDir (a session lives in exactly one file regardless of which
// encoded-cwd folder holds it). Unreadable subtrees are skipped; a missing
// transcript is an error the caller turns into "?".
func ResolveTranscript(projectsDir, sessionID string) (string, error) {
	if projectsDir == "" || sessionID == "" {
		return "", fmt.Errorf("cost: empty projectsDir or sessionID")
	}
	want := sessionID + ".jsonl"
	found := ""
	stop := errors.New("found")
	err := filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable subtree
		}
		if !d.IsDir() && d.Name() == want {
			found = path
			return stop
		}
		return nil
	})
	if found != "" {
		return found, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("cost: no transcript %s under %s", want, projectsDir)
}
