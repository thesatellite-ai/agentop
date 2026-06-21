package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/thesatellite-ai/agentop/internal/collect"
	"github.com/thesatellite-ai/agentop/internal/model"
)

func newReapCmd() *cobra.Command {
	var idleDays int
	var srcFilter string
	var yes bool

	cmd := &cobra.Command{
		Use:   "reap",
		Short: "Kill sessions idle past a threshold",
		Long: `reap terminates agent sessions whose idle time is at least --idle days.

Idle is measured from the session's controlling-tty I/O time (real activity),
so it is trustworthy for every source. Restrict to one controller with
--source emdash|cmux|direct. A confirmation lists exactly what will die.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := collect.Sessions()
			if err != nil {
				return err
			}
			want := model.Source(strings.ToLower(strings.TrimSpace(srcFilter)))
			if want != "" && want != model.SourceEmdash && want != model.SourceCmux && want != model.SourceDirect {
				return fmt.Errorf("invalid --source %q (want emdash|cmux|direct)", srcFilter)
			}

			var victims []model.Session
			for _, s := range sessions {
				if s.IdleDays() < idleDays {
					continue
				}
				if want != "" && s.Source != want {
					continue
				}
				victims = append(victims, s)
			}

			scope := "all sources"
			if want != "" {
				scope = string(want)
			}
			if len(victims) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Nothing idle ≥ %dd in %s.\n", idleDays, scope)
				return nil
			}

			fmt.Fprint(cmd.OutOrStdout(), renderTable(victims))
			if !yes {
				var total uint64
				for _, s := range victims {
					total += s.RSSBytes
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nKill %d session(s), reclaim ~%s? [y/N] ", len(victims), mem(total))
				reader := bufio.NewReader(os.Stdin)
				line, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(line)) != "y" {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			pids := make([]int, len(victims))
			for i, s := range victims {
				pids[i] = s.PID
			}
			if err := collect.KillAll(pids); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reaped %d session(s).\n", len(victims))
			return nil
		},
	}
	cmd.Flags().IntVar(&idleDays, "idle", model.DefaultReapIdleDays, "minimum idle days to reap")
	cmd.Flags().StringVar(&srcFilter, "source", "", "restrict to one source: emdash | cmux | direct")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
