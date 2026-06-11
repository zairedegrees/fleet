// Package relaymgr acquires and lifecycle-manages the agent-relay binary so a
// fleet user needs no separate wrai.th install. fleet (MIT) never embeds or
// redistributes the AGPL binary — it is downloaded (or built) on the user's
// behalf, with consent, on first use.
package relaymgr

import (
	"path/filepath"
	"strings"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

// probe is the reachability seam (swapped in tests).
var probe = defaultProbe

func defaultProbe(url string) error { return relay.NewClient(url).Health() }

// Reachable reports whether a relay answers at url.
func Reachable(url string) bool { return probe(url) == nil }

// Dir is ~/.fleet/bin, where the managed binary lives.
func Dir() string { return filepath.Join(config.FleetDir(), "bin") }

// BinPath is the managed agent-relay binary path.
func BinPath() string { return filepath.Join(Dir(), "agent-relay") }

func pidPath() string  { return filepath.Join(config.FleetDir(), "relay.pid") }
func lockPath() string { return filepath.Join(config.FleetDir(), "relay.lock") }

// filepathBase is a tiny indirection so the test reads clearly.
func filepathBase(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
