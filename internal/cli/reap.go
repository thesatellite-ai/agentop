package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
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
--source emdash|cmux|runbay|direct. A confirmation lists exactly what will die.

Each session gets SIGTERM, then up to 4s to exit; one still running after
that gets SIGKILL. Right before each signal agentop re-reads the process and
signals only when the pid still names the same session (same start time), so
a pid that ended or was reused since the list is skipped and reported as
"process changed or ended". Agent servers (claude mcp serve, codex app-server)
and the npm codex launcher are not sessions and are never reaped.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := collect.Sessions()
			if err != nil {
				return err
			}
			want := model.Source(strings.ToLower(strings.TrimSpace(srcFilter)))
			if want != "" && !slices.Contains(model.Sources, want) {
				return fmt.Errorf("invalid --source %q (want emdash|cmux|runbay|direct)", srcFilter)
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
			out := cmd.OutOrStdout()
			if len(victims) == 0 {
				_, err := fmt.Fprintf(out, "Nothing idle ≥ %dd in %s.\n", idleDays, scope)
				return err
			}

			// A failed write here aborts before anything is signalled: the
			// user must see exactly what dies before confirming.
			if _, err := fmt.Fprint(out, renderTable(victims)); err != nil {
				return err
			}
			if !yes {
				var total uint64
				for _, s := range victims {
					total += s.RSSBytes
				}
				if _, err := fmt.Fprintf(out, "\nKill %d session(s), reclaim ~%s? [y/N] ", len(victims), mem(total)); err != nil {
					return err
				}
				reader := bufio.NewReader(os.Stdin)
				line, _ := reader.ReadString('\n') // EOF or a read error reads as "no"
				if strings.TrimSpace(strings.ToLower(line)) != "y" {
					_, err := fmt.Fprintln(out, "Aborted.")
					return err
				}
			}
			return reapSessions(cmd.OutOrStdout(), victims, collect.NewReaper(collect.DefaultGrace))
		},
	}
	cmd.Flags().IntVar(&idleDays, "idle", model.DefaultReapIdleDays, "minimum idle days to reap")
	cmd.Flags().StringVar(&srcFilter, "source", "", "restrict to one source: emdash | cmux | runbay | direct")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// reapSessions stops victims through r (the same path the TUI uses: SIGTERM,
// grace, identity re-check, SIGKILL) and reports every session that was not
// cleanly stopped. It returns an error when any session failed or survived,
// joined with any error writing the report (the signals were already sent).
func reapSessions(w io.Writer, victims []model.Session, r collect.Reaper) error {
	targets := make([]collect.Target, len(victims))
	for i, s := range victims {
		targets[i] = collect.TargetOf(s)
	}
	results := r.Reap(targets)
	var writeErr error
	for _, res := range results {
		if res.Outcome == collect.OutcomeStopped {
			continue
		}
		line := fmt.Sprintf("pid %d: %s", res.PID, res.Outcome)
		if res.Err != nil {
			line += fmt.Sprintf(" (%v)", res.Err)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			writeErr = errors.Join(writeErr, err)
		}
	}
	sum := collect.Summarize(results)
	if _, err := fmt.Fprintf(w, "%s.\n", upperFirst(sum.String())); err != nil {
		writeErr = errors.Join(writeErr, err)
	}
	if sum.Failed > 0 || sum.Survived > 0 {
		return errors.Join(fmt.Errorf("%d session(s) could not be stopped", sum.Failed+sum.Survived), writeErr)
	}
	return writeErr
}

// upperFirst capitalises the first byte of an ASCII sentence.
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
