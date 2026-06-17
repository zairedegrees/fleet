package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/zairedegrees/fleet/internal/coordmgr"
	"github.com/zairedegrees/fleet/internal/runner"
)

// newCoordCmd is the hidden internal command tree. `fleet coord serve` is what
// the detached embedded-coord child runs; users don't invoke it directly.
func newCoordCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "coord", Short: "Embedded coordination core (internal)", Hidden: true}
	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the embedded coordination server (spawned by fleet)",
		RunE: func(_ *cobra.Command, _ []string) error {
			port := os.Getenv("PORT")
			if port == "" {
				port = "8090"
			}
			return coordmgr.Serve(port, func(project, agent, session string) (bool, error) {
				return runner.WakeSessionIfDormant(session, agent, project)
			})
		},
	})
	return cmd
}
