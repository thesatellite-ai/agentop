package cli

import (
	"bufio"
	"context"
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

Idle is the time since the session's last activity: the newer of its
controlling-tty I/O and the activity time the agent itself records (Claude's
session file), so it is trustworthy for every source. A session the agent
reports busy is never idle, so a threshold reap never picks it. Restrict to
one controller with --source emdash|cmux|runbay|direct. A confirmation lists exactly what will die.

What to signal comes from the agent's stop plan (agentio): each session gets
SIGTERM, then the agent's grace (4s for Claude and Codex) to exit; one still
running after that gets SIGKILL. Right before each signal agentop re-reads the
process table and signals only when the pid still names the same session
(same start time), so a pid that ended or was reused since the list is skipped
and reported as "process changed or ended". Agent servers (claude mcp serve,
codex app-server) and the npm codex launcher are not sessions and are never
listed or reaped; a Claude background session is refused with the claude
command that stops it. When the agent knows the conversation, the command
that resumes it is printed after the stop.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := collect.New()
			if err != nil {
				return err
			}
			sessions, err := c.Sessions(cmd.Context())
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
			return reapSessions(cmd.Context(), cmd.OutOrStdout(), victims, c.Reaper())
		},
	}
	cmd.Flags().IntVar(&idleDays, "idle", model.DefaultReapIdleDays, "minimum idle days to reap")
	cmd.Flags().StringVar(&srcFilter, "source", "", "restrict to one source: emdash | cmux | runbay | direct")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// reapSessions stops victims through r (the same path the TUI uses: the
// agent's stop plan, SIGTERM, grace, identity re-check, SIGKILL), reports
// every session that was not cleanly stopped, and prints the resume command
// of each stopped session whose conversation the agent knows. It returns an
// error when any session failed or survived, joined with any error writing
// the report (the signals were already sent).
func reapSessions(ctx context.Context, w io.Writer, victims []model.Session, r collect.Reaper) error {
	targets := make([]collect.Target, len(victims))
	for i, s := range victims {
		targets[i] = collect.TargetOf(s)
	}
	results := r.Reap(ctx, targets)
	var writeErr error
	for _, res := range results {
		line := resultLine(res)
		if line == "" {
			continue
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

// resultLine is the report line for one result, "" for a clean stop with
// nothing to resume.
func resultLine(res collect.Result) string {
	line := fmt.Sprintf("pid %d: %s", res.PID, res.Outcome)
	switch {
	case res.Reason != "":
		line += fmt.Sprintf(" (%s)", res.Reason)
	case res.Err != nil:
		line += fmt.Sprintf(" (%v)", res.Err)
	}
	if res.Resume != "" {
		return line + "; resume: " + res.Resume
	}
	if res.Outcome == collect.OutcomeStopped {
		return ""
	}
	return line
}

// upperFirst capitalises the first byte of an ASCII sentence.
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
