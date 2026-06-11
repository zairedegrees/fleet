package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zairedegrees/fleet/internal/relaymgr"
)

// seams for testing
var (
	reachable          = relaymgrReachable
	askConsent         = defaultAskConsent
	ensureRelayBinary  = relaymgr.EnsureBinary
	ensureRelaySkillFn = relaymgr.EnsureRelaySkill
	ensureRelayRunning = relaymgr.EnsureRunning
)

func relaymgrReachable(url string) bool { return relaymgr.Reachable(url) }

// ensureRelaySetup makes a relay available for a launch: if none is reachable and
// the url is the default local one, it asks consent once, then acquires the
// binary + /relay skill and auto-starts the managed relay. External --relay-url
// is never auto-managed.
func ensureRelaySetup(url string, isExternal bool) error {
	if reachable(url) {
		return nil
	}
	if isExternal {
		return fmt.Errorf("relay unreachable at %s (external --relay-url is not auto-managed)", url)
	}
	if !askConsent(url) {
		return fmt.Errorf("relay setup declined — install wrai.th manually or pass --relay-url <url>")
	}
	if _, err := ensureRelayBinary(); err != nil {
		return err
	}
	if err := ensureRelaySkillFn(); err != nil {
		return err
	}
	return ensureRelayRunning(url, isExternal)
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
	cmd := &cobra.Command{Use: "relay", Short: "Manage the fleet-bundled wrai.th relay"}
	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Download (first time) and start the managed relay",
		RunE: func(_ *cobra.Command, _ []string) error {
			url := resolveRelayURL(flagRelayURL, "")
			if relaymgr.Reachable(url) {
				fmt.Printf("  ✓ relay already running at %s\n", url)
				return nil
			}
			if err := ensureRelaySetup(url, flagRelayURL != ""); err != nil {
				return err
			}
			fmt.Printf("  ✓ relay started at %s\n", url)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the managed relay",
		RunE:  func(_ *cobra.Command, _ []string) error { return relaymgr.Stop() },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show managed-relay state",
		RunE: func(_ *cobra.Command, _ []string) error {
			url := resolveRelayURL(flagRelayURL, "")
			if relaymgr.Reachable(url) {
				fmt.Printf("  ✓ relay reachable at %s\n", url)
			} else {
				fmt.Printf("  ✗ no relay at %s (run 'fleet relay start')\n", url)
			}
			fmt.Printf("  binary: %s\n", relaymgr.BinPath())
			return nil
		},
	})
	return cmd
}
