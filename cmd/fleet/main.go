// cmd/fleet/main.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	flagLast   bool
	flagKill   bool
	flagStatus bool
	flagDoctor bool
)

func main() {
	root := &cobra.Command{
		Use:   "fleet",
		Short: "Launch multi-agent Claude Code fleets",
		RunE:  run,
	}

	root.Flags().BoolVar(&flagLast, "last", false, "Relaunch last saved config")
	root.Flags().BoolVar(&flagKill, "kill", false, "Stop all fleet tmux sessions")
	root.Flags().BoolVar(&flagStatus, "status", false, "List active fleet sessions")
	root.Flags().BoolVar(&flagDoctor, "doctor", false, "Check & install prerequisites")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	switch {
	case flagKill:
		fmt.Println("fleet --kill: not yet implemented")
	case flagStatus:
		fmt.Println("fleet --status: not yet implemented")
	case flagDoctor:
		fmt.Println("fleet --doctor: not yet implemented")
	case flagLast:
		fmt.Println("fleet --last: not yet implemented")
	default:
		fmt.Println("fleet wizard: not yet implemented")
	}
	return nil
}
