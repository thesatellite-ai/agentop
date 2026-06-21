package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/thesatellite-ai/agentop/internal/collect"
)

func newListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List running agent sessions (one-shot, no TUI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := collect.Sessions()
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(sessions)
			}
			fmt.Fprint(cmd.OutOrStdout(), renderTable(sessions))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output machine-readable JSON")
	return cmd
}
