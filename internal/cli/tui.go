package cli

import "github.com/thesatellite-ai/agentop/internal/tui"

// runTUI launches the interactive terminal UI (default action).
func runTUI() error {
	return tui.Run()
}
