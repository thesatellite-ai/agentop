// Package cli wires the cobra command tree for the agentop binary.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// BinaryName is how this tool appears on PATH and in every help string.
// Renaming is one constant edit + rebuild.
const BinaryName = "agentop"

// version is set at build time via -ldflags '-X .../cli.version=<value>'.
var version = "0.1.0-dev"

// SetVersion lets main inject the build-time version.
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   BinaryName,
		Short: "agentop — htop for AI coding agents",
		Long: `agentop finds every running AI coding-agent session across cmux,
emdash, and plain terminals — showing memory, idle age, and which task each
belongs to — and lets you selectively cull the idle ones to reclaim RAM.

Run with no arguments to open the interactive TUI.`,
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		// Default action (no subcommand) launches the TUI.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI()
		},
	}
	root.AddCommand(newListCmd())
	root.AddCommand(newReapCmd())
	root.AddCommand(newVersionCmd())
	return root
}

// Execute runs the root command.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
