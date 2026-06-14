package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/coordmgr"
	"github.com/zairedegrees/fleet/internal/relaymgr"
)

const (
	backendEmbedded = "embedded"
	backendDownload = "download"
	// defaultBackend is used when nothing overrides it: the native in-binary coord
	// (MIT, no download). The AGPL agent-relay binary remains available as an
	// explicit opt-in via --relay-backend download / FLEET_RELAY_BACKEND=download.
	defaultBackend = backendEmbedded
)

// seams for testing
var (
	reachable          = relaymgrReachable
	askConsent         = defaultAskConsent
	ensureRelayBinary  = relaymgr.EnsureBinary
	ensureRelaySkillFn = relaymgr.EnsureRelaySkill
	ensureRelayRunning = relaymgr.EnsureRunning
	installCoordSkill  = coordmgr.InstallSkill
	ensureCoordRunning = coordmgr.EnsureRunning
)

func relaymgrReachable(url string) bool { return relaymgr.Reachable(url) }

// coordBackend resolves the coordination backend: --relay-backend flag >
// FLEET_RELAY_BACKEND env > the project's relay_backend > the built-in default.
func coordBackend(cfg *config.FleetConfig) string {
	if v := strings.TrimSpace(flagRelayBackend); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("FLEET_RELAY_BACKEND")); v != "" {
		return v
	}
	if cfg != nil && cfg.Project.RelayBackend != "" {
		return strings.TrimSpace(cfg.Project.RelayBackend)
	}
	return defaultBackend
}

// resolvedBackend resolves the backend for standalone commands (relay
// start/status, doctor) that have no launch cfg in hand, best-effort consulting
// the last project's config so a config-pinned backend is honored everywhere the
// launch path honors it.
func resolvedBackend() string {
	cfg, err := config.LoadLast()
	if err != nil {
		cfg = nil
	}
	return coordBackend(cfg)
}

// stopBackends stops both managed backends; each is a no-op without its pidfile.
// Both always run (so one failing doesn't strand the other) and errors combine.
func stopBackends() error {
	return errors.Join(coordmgr.Stop(), relaymgr.Stop())
}

// ensureRelaySetup makes a relay available for a launch. If none is reachable and
// the url is the default local one, it brings up the selected backend: "embedded"
// starts the in-binary coord (no download, no AGPL, no consent), "download"
// acquires the AGPL agent-relay binary after consent. External --relay-url is
// never auto-managed.
func ensureRelaySetup(url string, isExternal bool, backend string) error {
	// Installing the /relay skill is a launch prerequisite, NOT part of starting
	// the server: an agent woken later needs it to resolve `/relay talk`. So for
	// the embedded backend it must happen even when coord is already reachable
	// (a persistent service or a prior launch) — it can't sit behind the
	// reachability short-circuit. The install is idempotent.
	if backend == backendEmbedded && !isExternal {
		if err := installCoordSkill(); err != nil {
			return err
		}
	}
	if reachable(url) {
		return nil
	}
	if isExternal {
		return fmt.Errorf("relay unreachable at %s (external --relay-url is not auto-managed)", url)
	}
	switch backend {
	case backendEmbedded:
		return ensureCoordRunning(url, isExternal)
	case backendDownload:
		if !askConsent(url) {
			return fmt.Errorf("relay setup declined — install wrai.th manually, pass --relay-url <url>, or use --relay-backend embedded")
		}
		if _, err := ensureRelayBinary(); err != nil {
			return err
		}
		if err := ensureRelaySkillFn(); err != nil {
			return err
		}
		return ensureRelayRunning(url, isExternal)
	default:
		return fmt.Errorf("unknown relay backend %q (valid: %q or %q)", backend, backendEmbedded, backendDownload)
	}
}

func defaultAskConsent(url string) bool {
	fmt.Printf("  fleet needs a wrai.th relay and none is running at %s.\n", url)
	fmt.Println("  It will download the agent-relay binary (AGPL-3.0, from Synergix-lab/WRAI.TH)")
	fmt.Println("  and the /relay skill, into ~/.fleet and ~/.claude/skills, and run it locally.")
	fmt.Print("  Proceed? [y/N] ")
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes"
}

func newRelayCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "relay", Short: "Manage the coordination backend (embedded coord or downloaded relay)"}
	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the coordination backend (embedded coord, or the downloaded relay)",
		RunE: func(_ *cobra.Command, _ []string) error {
			url := resolveRelayURL(flagRelayURL, "")
			if relaymgr.Reachable(url) {
				fmt.Printf("  ✓ relay already running at %s\n", url)
				return nil
			}
			backend := resolvedBackend()
			if err := ensureRelaySetup(url, flagRelayURL != "", backend); err != nil {
				return err
			}
			fmt.Printf("  ✓ relay started at %s (%s)\n", url, backend)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the managed coordination backend",
		RunE:  func(_ *cobra.Command, _ []string) error { return stopBackends() },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show coordination-backend state",
		RunE: func(_ *cobra.Command, _ []string) error {
			url := resolveRelayURL(flagRelayURL, "")
			if relaymgr.Reachable(url) {
				fmt.Printf("  ✓ relay reachable at %s\n", url)
			} else {
				fmt.Printf("  ✗ no relay at %s (run 'fleet relay start')\n", url)
			}
			backend := resolvedBackend()
			fmt.Printf("  backend: %s\n", backend)
			if backend == backendDownload {
				fmt.Printf("  binary: %s\n", relaymgr.BinPath())
			} else {
				fmt.Printf("  db: %s\n", coordmgr.DBPath())
			}
			return nil
		},
	})
	return cmd
}
