package cli

import (
	"context"

	"github.com/thesatellite-ai/agentop/internal/collect"
	"github.com/thesatellite-ai/agentop/internal/tui"
)

// runTUI launches the interactive terminal UI (default action).
func runTUI(ctx context.Context) error {
	c, err := collect.New()
	if err != nil {
		return err
	}
	return tui.Run(ctx, c)
}
