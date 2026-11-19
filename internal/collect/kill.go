package collect

import (
	"errors"
	"fmt"
	"syscall"
	"time"

	"github.com/thesatellite-ai/agentop/internal/agent"
	"github.com/thesatellite-ai/agentop/internal/discover"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// DefaultGrace is how long a SIGTERM'd session gets to flush and exit before
// it is escalated to SIGKILL. Generous enough for Claude to checkpoint. The TUI
// and the CLI both use it, so both kill the same way.
const DefaultGrace = 4 * time.Second

const (
	// defaultPoll is how often the reaper re-reads the targets while waiting.
	defaultPoll = 100 * time.Millisecond
	// defaultKillWait is how long to wait for SIGKILL to take effect; it is
	// delivered asynchronously too.
	defaultKillWait = time.Second
)

// Target is one process to stop: the pid plus the start time seen when it
// was listed. The start time is the identity check — a pid reused by an
// unrelated process has a different start time and is never signalled.
type Target struct {
	PID       int
	StartedAt time.Time
}

// TargetOf returns the kill target for a listed session.
func TargetOf(s model.Session) Target {
	return Target{PID: s.PID, StartedAt: s.StartedAt}
}

// Outcome is what happened to one target.
type Outcome string

const (
	// OutcomeStopped: exited after SIGTERM within the grace period.
	OutcomeStopped Outcome = "stopped"
	// OutcomeForced: still the same process after the grace period, so it
	// got SIGKILL and exited.
	OutcomeForced Outcome = "killed after grace"
	// OutcomeChanged: before a signal the pid had ended or now names another
	// process (different start time, or no longer an agent session), so it
	// was not signalled.
	OutcomeChanged Outcome = "process changed or ended"
	// OutcomeSurvived: still the same live process after SIGKILL.
	OutcomeSurvived Outcome = "survived"
	// OutcomeFailed: the identity check or a signal failed (for example ps
	// could not run, or EPERM); the process was not confirmed and not killed.
	OutcomeFailed Outcome = "failed"
)

// Result is the outcome for one target.
type Result struct {
	Target
	Outcome Outcome
	Err     error
}

// Gone reports whether the target no longer runs as the listed process.
func (r Result) Gone() bool {
	return r.Outcome == OutcomeStopped || r.Outcome == OutcomeForced || r.Outcome == OutcomeChanged
}

// ProcLookup returns the current process rows for pids; ended pids are absent.
type ProcLookup func(pids []int) (map[int]discover.Proc, error)

// Signaler delivers sig to pid.
type Signaler func(pid int, sig syscall.Signal) error

// Reaper stops sessions: SIGTERM, wait up to Grace, SIGKILL the survivors.
// Before every signal it re-reads each target and signals only when the pid
// still names the same live agent session (same start time). Lookup, Signal
// and Sleep are injectable so tests use fake process tables and never signal
// a real process.
type Reaper struct {
	Lookup   ProcLookup
	Signal   Signaler
	Sleep    func(time.Duration)
	Grace    time.Duration
	Poll     time.Duration
	KillWait time.Duration
}

// NewReaper returns a Reaper on the real process table and kill(2).
func NewReaper(grace time.Duration) Reaper {
	return Reaper{
		Lookup:   discover.LookupProcs,
		Signal:   syscall.Kill,
		Sleep:    time.Sleep,
		Grace:    grace,
		Poll:     defaultPoll,
		KillWait: defaultKillWait,
	}
}

// Reap stops targets on the real process table: SIGTERM, wait grace,
// re-check, SIGKILL. It blocks until each target is confirmed gone, has survived
// SIGKILL, or was skipped.
func Reap(targets []Target, grace time.Duration) []Result {
	return NewReaper(grace).Reap(targets)
}

// Reap runs the stop sequence and returns one Result per target, in order.
func (r Reaper) Reap(targets []Target) []Result {
	results := make([]Result, len(targets))
	for i, t := range targets {
		results[i] = Result{Target: t}
	}
	if len(targets) == 0 {
		return results
	}

	// Identity check, then SIGTERM.
	rows, err := r.Lookup(pidsOf(targets))
	if err != nil {
		for i := range results {
			results[i].Outcome = OutcomeFailed
			results[i].Err = fmt.Errorf("cannot confirm process identity: %w", err)
		}
		return results
	}
	var pending []int // indexes of targets that got SIGTERM
	for i, t := range targets {
		if !sameProcess(t, rows) {
			results[i].Outcome = OutcomeChanged
			continue
		}
		if done := r.signal(&results[i], syscall.SIGTERM); done {
			continue
		}
		pending = append(pending, i)
	}

	// Wait up to Grace for them to exit.
	pending = r.waitGone(results, pending, r.Grace, OutcomeStopped)
	if len(pending) == 0 {
		return results
	}

	// Re-check identity right before SIGKILL: a target that exited during the
	// grace period may already have its pid reused.
	rows, err = r.Lookup(pidsOf(subsetResults(results, pending)))
	if err != nil {
		for _, i := range pending {
			results[i].Outcome = OutcomeFailed
			results[i].Err = fmt.Errorf("cannot confirm process identity before SIGKILL: %w", err)
		}
		return results
	}
	var forced []int
	for _, i := range pending {
		if !sameProcess(targets[i], rows) {
			results[i].Outcome = OutcomeStopped // exited during the grace period
			continue
		}
		if done := r.signal(&results[i], syscall.SIGKILL); done {
			continue
		}
		forced = append(forced, i)
	}
	forced = r.waitGone(results, forced, r.KillWait, OutcomeForced)
	for _, i := range forced {
		results[i].Outcome = OutcomeSurvived
	}
	return results
}

// signal sends sig and records a terminal outcome when it cannot be sent.
// It returns true when the target needs no further waiting.
func (r Reaper) signal(res *Result, sig syscall.Signal) bool {
	err := r.Signal(res.PID, sig)
	switch {
	case err == nil:
		return false
	case errors.Is(err, syscall.ESRCH):
		res.Outcome = OutcomeChanged // ended between the check and the signal
	default:
		res.Outcome = OutcomeFailed
		res.Err = fmt.Errorf("%s pid %d: %w", sig, res.PID, err)
	}
	return true
}

// waitGone polls pending targets for up to limit. Each that is no longer the
// same process gets outcome gone; the rest are returned. A failed lookup is
// treated as "unknown" for that round and polling continues.
func (r Reaper) waitGone(results []Result, pending []int, limit time.Duration, gone Outcome) []int {
	poll := max(r.Poll, time.Millisecond)
	for waited := time.Duration(0); len(pending) > 0; waited += poll {
		rows, err := r.Lookup(pidsOf(subsetResults(results, pending)))
		if err == nil {
			var still []int
			for _, i := range pending {
				if sameProcess(results[i].Target, rows) {
					still = append(still, i)
				} else {
					results[i].Outcome = gone
				}
			}
			pending = still
		}
		if len(pending) == 0 || waited >= limit {
			break
		}
		r.Sleep(poll)
	}
	return pending
}

// sameProcess reports whether rows still holds t as the listed process: the
// pid exists, its start time matches, and it is still an agent session. An
// unknown start time can never be confirmed, so it is never the same.
func sameProcess(t Target, rows map[int]discover.Proc) bool {
	p, ok := rows[t.PID]
	if !ok || t.StartedAt.IsZero() || !p.StartedAt.Equal(t.StartedAt) {
		return false
	}
	return agent.IsSession(p.Command)
}

// pidsOf lists the pids of ts in order.
func pidsOf(ts []Target) []int {
	pids := make([]int, len(ts))
	for i, t := range ts {
		pids[i] = t.PID
	}
	return pids
}

// subsetResults returns the targets of results at idx.
func subsetResults(rs []Result, idx []int) []Target {
	out := make([]Target, len(idx))
	for i, j := range idx {
		out[i] = rs[j].Target
	}
	return out
}

// Summary counts results by outcome for a one-line status message.
type Summary struct {
	Stopped, Forced, Changed, Survived, Failed int
}

// Summarize counts results by outcome.
func Summarize(results []Result) Summary {
	var s Summary
	for _, r := range results {
		switch r.Outcome {
		case OutcomeStopped:
			s.Stopped++
		case OutcomeForced:
			s.Forced++
		case OutcomeChanged:
			s.Changed++
		case OutcomeSurvived:
			s.Survived++
		default:
			s.Failed++
		}
	}
	return s
}

// String renders the summary, naming only the non-zero outcomes.
func (s Summary) String() string {
	out := fmt.Sprintf("reaped %d", s.Stopped+s.Forced)
	if s.Forced > 0 {
		out += fmt.Sprintf(" (%d after SIGKILL)", s.Forced)
	}
	if s.Changed > 0 {
		out += fmt.Sprintf(", %d skipped: %s", s.Changed, OutcomeChanged)
	}
	if s.Survived > 0 {
		out += fmt.Sprintf(", %d survived SIGKILL", s.Survived)
	}
	if s.Failed > 0 {
		out += fmt.Sprintf(", %d failed", s.Failed)
	}
	return out
}
