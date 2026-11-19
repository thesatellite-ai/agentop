package collect

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/ubgo/agentio"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// DefaultGrace is how long a SIGTERM'd session gets to flush and exit before
// it is escalated to SIGKILL when the agent's stop plan names no grace of its
// own. agentio's plans use the same 4s for Claude and Codex. The TUI and the
// CLI both reap through Reaper, so both kill the same way.
const DefaultGrace = agentio.DefaultStopGrace

const (
	// defaultPoll is how often the reaper re-reads the targets while waiting.
	defaultPoll = 100 * time.Millisecond
	// defaultKillWait is how long to wait for SIGKILL to take effect; it is
	// delivered asynchronously too.
	defaultKillWait = time.Second
)

// Target is one session to stop: the pid plus the start time seen when it
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
	// OutcomeStopped: exited after the graceful signal within the grace period.
	OutcomeStopped Outcome = "stopped"
	// OutcomeForced: still the same process after the grace period, so it
	// got SIGKILL and exited.
	OutcomeForced Outcome = "killed after grace"
	// OutcomeChanged: before a signal the pid had ended or now names another
	// process (different start time, or no longer an agent session), so it
	// was not signalled.
	OutcomeChanged Outcome = "process changed or ended"
	// OutcomeRefused: the agent's stop plan says this process must not be
	// signalled (not a conversation process, or the agent's own stop command
	// must end it); Result.Reason says why. Nothing was signalled.
	OutcomeRefused Outcome = "refused"
	// OutcomeSurvived: still the same live process after SIGKILL, or after
	// the grace period when the plan says forcing is unsafe.
	OutcomeSurvived Outcome = "survived"
	// OutcomeFailed: the identity check or a signal failed (for example the
	// process table could not be read, or EPERM); the process was not
	// confirmed and not killed.
	OutcomeFailed Outcome = "failed"
)

// Result is the outcome for one target.
type Result struct {
	Target
	Outcome Outcome
	Err     error
	// Reason is the plan's refusal, set with OutcomeRefused.
	Reason string
	// Resume is the line a person types to bring the conversation back
	// (agentio StopPlan.ResumeLine: the home variable for a non-default
	// home, never a token); "" when the agent does not know the
	// conversation. Set with OutcomeStopped and OutcomeForced.
	Resume string
}

// Gone reports whether the target no longer runs as the listed process.
func (r Result) Gone() bool {
	return r.Outcome == OutcomeStopped || r.Outcome == OutcomeForced || r.Outcome == OutcomeChanged
}

// TableReader reads the whole process table once (agentio Host.Processes).
type TableReader func(ctx context.Context) ([]agentio.Process, error)

// Classifier says which agent a row is and whether it is a conversation
// process (agentio.ClassifyProcess over the agents).
type Classifier func(p agentio.Process) (agentio.ProcessInfo, bool)

// Planner returns the agent's stop plan for row p of table. ok is false when
// p is not an agent conversation process any more.
type Planner func(ctx context.Context, p agentio.Process, table []agentio.Process) (plan agentio.StopPlan, ok bool)

// AliveChecker re-checks recorded processes with one process-table read
// (agentio Host.ProcessesAlive), one answer per ref in order.
type AliveChecker func(ctx context.Context, refs []agentio.ProcessRef) ([]agentio.Liveness, error)

// Signaler delivers sig to pid.
type Signaler func(pid int, sig syscall.Signal) error

// Reaper stops sessions. agentio decides what to signal (the stop plan:
// targets, graceful signal, grace, whether forcing is safe, or a refusal);
// the Reaper sends the signals: graceful signal, wait up to the grace, force
// the survivors. Before every signal it re-reads the process table and
// signals only a pid that still names the same live process (same start
// time, still the agent process it was). Table, Alive, Classify, Plan,
// Signal, Sleep and Now are injectable so tests use fake process tables and never signal a
// real process.
type Reaper struct {
	Table TableReader
	// Alive polls the targets while they get their grace: a target is done
	// once agentio says every pid of it ended. It is never the check before
	// a signal; that is the exact re-check on a fresh Table read.
	Alive    AliveChecker
	Classify Classifier
	Plan     Planner
	Signal   Signaler
	Sleep    func(time.Duration)
	// Now is the clock the grace is measured on: the wall time a poll
	// round takes (a full process-table read) counts toward the grace.
	Now      func() time.Time
	Poll     time.Duration
	KillWait time.Duration
}

// Reaper returns the Reaper on the real process table and kill(2), planning
// with the collector's agents.
func (c *Collector) Reaper() Reaper {
	return Reaper{
		Table:    c.table,
		Alive:    c.Host.ProcessesAlive,
		Classify: func(p agentio.Process) (agentio.ProcessInfo, bool) { return agentio.ClassifyProcess(c.Agents, p) },
		Plan:     c.plan,
		Signal:   syscall.Kill,
		Sleep:    time.Sleep,
		Now:      time.Now,
		Poll:     defaultPoll,
		KillWait: defaultKillWait,
	}
}

// plan asks p's agent for its stop plan, with what the agent recorded about
// the process (for the resume command and a background session's stop
// command). ok is false when p is not an agent conversation process.
func (c *Collector) plan(ctx context.Context, p agentio.Process, table []agentio.Process) (agentio.StopPlan, bool) {
	info, ok := agentio.ClassifyProcess(c.Agents, p)
	if !ok || !info.Session {
		return agentio.StopPlan{}, false
	}
	a := agentByID(c.Agents, info.Agent)
	planner, ok := agentio.StopPlannerOf(a)
	if !ok {
		return agentio.StopPlan{Agent: info.Agent, Refuse: refuseNoPlanner}, true
	}
	// Best effort: without the join the plan still stops the process, it
	// just cannot name the resume command.
	ps, _, _ := agentio.FindProcessSession(ctx, c.Agents, p)
	plan, err := planner.StopPlan(p, table, ps)
	if err != nil {
		return agentio.StopPlan{}, false
	}
	return plan, true
}

// refuseNoPlanner is the refusal for an agent without a stop plan: agentop
// only signals what the agent says is safe to signal.
const refuseNoPlanner = "the agent has no stop plan"

// refuseCommand prefixes the refusal for a session the agent's own command
// must end (Claude background sessions: a supervisor respawns a killed worker).
const refuseCommand = "stop it with the agent's command: "

// refuseEmpty is the refusal for a plan that names no pid.
const refuseEmpty = "the stop plan names no process"

// proc is one pid a job signals, with the start time it had when the plan
// was made; session marks the listed conversation process itself.
type proc struct {
	pid     int
	start   time.Time
	session bool
	gone    bool
}

// job is one target being stopped by its plan.
type job struct {
	idx   int // index into results
	plan  agentio.StopPlan
	procs []proc
}

// Reap runs the stop sequence and returns one Result per target, in order.
func (r Reaper) Reap(ctx context.Context, targets []Target) []Result {
	results := make([]Result, len(targets))
	for i, t := range targets {
		results[i] = Result{Target: t}
	}
	if len(targets) == 0 {
		return results
	}

	// Identity check and plan, from one table read.
	table, err := r.Table(ctx)
	if err != nil {
		for i := range results {
			results[i].Outcome = OutcomeFailed
			results[i].Err = fmt.Errorf("cannot confirm process identity: %w", err)
		}
		return results
	}
	jobs := r.plan(ctx, results, table)

	// Graceful signal to every pid of every job.
	var waiting, force []*job
	for _, j := range jobs {
		sig, ok := gracefulSignal(j.plan.Graceful)
		if !ok {
			force = append(force, j) // no graceful request exists: force only
			continue
		}
		if r.signalJob(j, &results[j.idx], sig) {
			waiting = append(waiting, j)
		}
	}

	// Wait up to each plan's grace for its pids to exit.
	force = append(force, r.waitGone(ctx, results, waiting, graceOf, OutcomeStopped)...)
	if len(force) == 0 {
		return results
	}

	// Re-check identity right before SIGKILL: a target that exited during the
	// grace period may already have its pid reused.
	table, err = r.Table(ctx)
	if err != nil {
		for _, j := range force {
			results[j.idx].Outcome = OutcomeFailed
			results[j.idx].Err = fmt.Errorf("cannot confirm process identity before SIGKILL: %w", err)
		}
		return results
	}
	var killed []*job
	for _, j := range force {
		res := &results[j.idx]
		if !r.refresh(j, table) {
			res.Outcome = OutcomeStopped // exited during the grace period
			res.Resume = j.plan.ResumeLine()
			continue
		}
		if !j.plan.Force {
			res.Outcome = OutcomeSurvived
			res.Err = errForceUnsafe
			continue
		}
		if r.signalJob(j, res, syscall.SIGKILL) {
			killed = append(killed, j)
		}
	}
	for _, j := range r.waitGone(ctx, results, killed, func(*job) time.Duration { return r.KillWait }, OutcomeForced) {
		results[j.idx].Outcome = OutcomeSurvived
	}
	return results
}

// errForceUnsafe is the error for a survivor the plan says not to force.
var errForceUnsafe = errors.New("still running after the grace period; the agent's stop plan says forcing it is unsafe")

// plan confirms each target against table and builds its job. A target
// whose pid ended, was reused, or is no longer an agent conversation gets
// OutcomeChanged; a refused plan gets OutcomeRefused. Neither is signalled.
func (r Reaper) plan(ctx context.Context, results []Result, table []agentio.Process) []*job {
	byPID := indexTable(table)
	var jobs []*job
	for i := range results {
		res := &results[i]
		row, ok := byPID[res.PID]
		if !ok || !sameStart(res.StartedAt, row.Start) {
			res.Outcome = OutcomeChanged
			continue
		}
		plan, ok := r.Plan(ctx, row, table)
		if !ok {
			res.Outcome = OutcomeChanged
			continue
		}
		if reason := refusal(plan); reason != "" {
			res.Outcome = OutcomeRefused
			res.Reason = reason
			continue
		}
		j := &job{idx: i, plan: plan}
		for _, pid := range plan.Targets {
			p, ok := byPID[pid]
			// A pid the plan adds (Gemini's relaunch parent) is signalled only
			// when this same read gives it a start time to re-check later and
			// it is an agent process itself.
			if !ok || p.Start.IsZero() {
				continue
			}
			pr := proc{pid: pid, start: p.Start, session: pid == res.PID}
			if !r.sameProcess(pr, p) {
				continue
			}
			j.procs = append(j.procs, pr)
		}
		jobs = append(jobs, j)
	}
	return jobs
}

// refusal is why a plan must not be carried out by a signal, "" when it can.
func refusal(plan agentio.StopPlan) string {
	switch {
	case plan.Refuse != "":
		return plan.Refuse
	case plan.Command != nil:
		return refuseCommand + strings.Join(append([]string{plan.Command.Path}, plan.Command.Args...), " ")
	case len(plan.Targets) == 0:
		return refuseEmpty
	}
	return ""
}

// signalJob sends sig to every live pid of j, in plan order. It returns true
// when j needs waiting for; a failed signal records OutcomeFailed, and a job
// whose pids all ended before the signal records OutcomeChanged.
func (r Reaper) signalJob(j *job, res *Result, sig syscall.Signal) bool {
	sent := false
	for k := range j.procs {
		p := &j.procs[k]
		if p.gone {
			continue
		}
		err := r.Signal(p.pid, sig)
		switch {
		case err == nil:
			sent = true
		case errors.Is(err, syscall.ESRCH):
			p.gone = true // ended between the check and the signal
		default:
			res.Outcome = OutcomeFailed
			res.Err = fmt.Errorf("%s pid %d: %w", sig, p.pid, err)
			return false
		}
	}
	if !sent && res.Outcome == "" {
		res.Outcome = OutcomeChanged
	}
	return sent
}

// waitGone polls jobs, each for up to limit(job), with agentio's batch
// liveness check (one table read per round). A job whose pids all ended
// gets outcome gone; the jobs still running when their limit passes are
// returned. A pid agentio cannot confirm either way (LivenessUnknown) or a
// failed read counts as still running: waiting longer is always safe, and
// the exact re-check before SIGKILL decides what is signalled.
func (r Reaper) waitGone(ctx context.Context, results []Result, jobs []*job, limit func(*job) time.Duration, gone Outcome) []*job {
	poll := max(r.Poll, time.Millisecond)
	began := r.Now()
	var over []*job
	for len(jobs) > 0 {
		r.markEnded(ctx, jobs)
		var still []*job
		for _, j := range jobs {
			if j.running() {
				still = append(still, j)
				continue
			}
			results[j.idx].Outcome = gone
			results[j.idx].Resume = j.plan.ResumeLine()
		}
		jobs = still
		waited := r.Now().Sub(began)
		var next []*job
		for _, j := range jobs {
			if waited >= limit(j) {
				over = append(over, j)
			} else {
				next = append(next, j)
			}
		}
		jobs = next
		if len(jobs) == 0 {
			break
		}
		r.Sleep(poll)
	}
	return over
}

// markEnded asks agentio once about every live pid of jobs and marks the
// ones that ended (gone, or the pid now has another start time).
func (r Reaper) markEnded(ctx context.Context, jobs []*job) {
	var refs []agentio.ProcessRef
	var procs []*proc
	for _, j := range jobs {
		for k := range j.procs {
			if p := &j.procs[k]; !p.gone {
				refs = append(refs, agentio.ProcessRef{PID: p.pid, Start: p.start})
				procs = append(procs, p)
			}
		}
	}
	if len(refs) == 0 {
		return
	}
	alive, err := r.Alive(ctx, refs)
	if err != nil || len(alive) != len(refs) {
		return // unknown this round: keep waiting
	}
	for i, l := range alive {
		if l == agentio.LivenessEnded {
			procs[i].gone = true
		}
	}
}

// running reports whether any pid of j is not known to have ended.
func (j *job) running() bool {
	for _, p := range j.procs {
		if !p.gone {
			return true
		}
	}
	return false
}

// graceOf is the plan's grace, DefaultGrace when it names none.
func graceOf(j *job) time.Duration {
	if j.plan.Grace > 0 {
		return j.plan.Grace
	}
	return DefaultGrace
}

// refresh marks each pid of j that no longer names the same process in table
// as gone, and reports whether any is still running.
func (r Reaper) refresh(j *job, table []agentio.Process) bool {
	byPID := indexTable(table)
	alive := false
	for k := range j.procs {
		p := &j.procs[k]
		if p.gone {
			continue
		}
		if row, ok := byPID[p.pid]; ok && r.sameProcess(*p, row) {
			alive = true
			continue
		}
		p.gone = true
	}
	return alive
}

// sameProcess reports whether row is still p: same start time, and still the
// agent process it was (the session pid must still be a conversation
// process, so a pid that exec'd into another program is never signalled).
func (r Reaper) sameProcess(p proc, row agentio.Process) bool {
	if !sameStart(p.start, row.Start) {
		return false
	}
	info, ok := r.Classify(row)
	return ok && (!p.session || info.Session)
}

// sameStart reports equal, known start times. Both come from the same
// process-table reader, so the same process always reads the same value; an
// unknown start time can never be confirmed, so it is never the same.
func sameStart(want, got time.Time) bool {
	return !want.IsZero() && !got.IsZero() && got.Equal(want)
}

// gracefulSignal maps the plan's graceful request to a unix signal; false
// when the plan has none (then the only way is to force).
func gracefulSignal(s agentio.StopSignal) (syscall.Signal, bool) {
	switch s {
	case agentio.StopSignalTerminate:
		return syscall.SIGTERM, true
	case agentio.StopSignalInterrupt:
		return syscall.SIGINT, true
	}
	return 0, false
}

// indexTable maps a process table by pid.
func indexTable(table []agentio.Process) map[int]agentio.Process {
	byPID := make(map[int]agentio.Process, len(table))
	for _, p := range table {
		byPID[p.PID] = p
	}
	return byPID
}

// agentByID returns the agent with id; nil when none.
func agentByID(agents []agentio.Agent, id agentio.ID) agentio.Agent {
	for _, a := range agents {
		if a.ID() == id {
			return a
		}
	}
	return nil
}

// Summary counts results by outcome for a one-line status message.
type Summary struct {
	Stopped, Forced, Changed, Refused, Survived, Failed int
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
		case OutcomeRefused:
			s.Refused++
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
	if s.Refused > 0 {
		out += fmt.Sprintf(", %d refused by the agent's stop plan", s.Refused)
	}
	if s.Survived > 0 {
		out += fmt.Sprintf(", %d survived", s.Survived)
	}
	if s.Failed > 0 {
		out += fmt.Sprintf(", %d failed", s.Failed)
	}
	return out
}
