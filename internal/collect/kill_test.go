package collect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// fakeOS is a scripted process table plus a signal recorder. No test here
// touches a real process: Signal only records and mutates the fake table.
type fakeOS struct {
	procs   map[int]agentio.Process
	signals []sent
	// onSignal lets a case decide what a signal does to the fake table.
	onSignal func(f *fakeOS, pid int, sig syscall.Signal) error
	// onRead runs before each table read (count starts at 1); it can end,
	// replace or fail processes over time.
	onRead     func(f *fakeOS, n int) error
	reads      int
	sleptTotal time.Duration
}

type sent struct {
	pid int
	sig syscall.Signal
}

func (f *fakeOS) table(context.Context) ([]agentio.Process, error) {
	f.reads++
	if f.onRead != nil {
		if err := f.onRead(f, f.reads); err != nil {
			return nil, err
		}
	}
	out := make([]agentio.Process, 0, len(f.procs))
	for _, p := range f.procs {
		out = append(out, p)
	}
	return out, nil
}

// Processes makes f an agentio.ProcessLister, so agentio's batch liveness
// check (Host.ProcessesAlive) reads the same scripted table.
func (f *fakeOS) Processes(ctx context.Context) ([]agentio.Process, error) { return f.table(ctx) }

// Cwd is unused by the reaper; it completes agentio.ProcessLister.
func (f *fakeOS) Cwd(context.Context, int) (string, error) { return "", errors.New("no cwd") }

// alive is agentio's real batch liveness check on f's table.
func (f *fakeOS) alive(ctx context.Context, refs []agentio.ProcessRef) ([]agentio.Liveness, error) {
	return agentio.Host{Processes: f}.ProcessesAlive(ctx, refs, agentio.ExactStart())
}

func (f *fakeOS) signal(pid int, sig syscall.Signal) error {
	f.signals = append(f.signals, sent{pid, sig})
	if f.onSignal != nil {
		return f.onSignal(f, pid, sig)
	}
	return nil
}

// Poll cadence of the test reaper. With agentio's 4s plan grace the reads
// are: 1 = pre-signal check and plan, 2..graceReads+1 = the grace polls,
// graceReads+2 = the pre-SIGKILL check.
const (
	testPoll     = 100 * time.Millisecond
	testKillWait = 300 * time.Millisecond
	graceReads   = int(DefaultGrace/testPoll) + 1
	preKillRead  = graceReads + 2
)

// reaper returns a Reaper whose classification and stop plans are agentio's
// real ones (built-in agents on a fixture host) and whose process table,
// signals and clock are f's.
func (f *fakeOS) reaper(t *testing.T) Reaper {
	t.Helper()
	r := testCollector(t, &agenttest.FakeProcesses{}).Reaper()
	r.Table = f.table
	r.Alive = f.alive
	r.Signal = f.signal
	r.Sleep = func(d time.Duration) { f.sleptTotal += d }
	r.Now = func() time.Time { return t0.Add(f.sleptTotal) }
	r.Poll = testPoll
	r.KillWait = testKillWait
	return r
}

// procs builds a fake table from rows.
func procs(rows ...agentio.Process) map[int]agentio.Process {
	m := map[int]agentio.Process{}
	for _, p := range rows {
		m[p.PID] = p
	}
	return m
}

// exitOn makes the fake process exit when it receives one of sigs.
func exitOn(sigs ...syscall.Signal) func(*fakeOS, int, syscall.Signal) error {
	return func(f *fakeOS, pid int, sig syscall.Signal) error {
		if _, ok := f.procs[pid]; !ok {
			return syscall.ESRCH
		}
		if slices.Contains(sigs, sig) {
			delete(f.procs, pid)
		}
		return nil
	}
}

const (
	aliceSession = "claude --resume abc"
	bobSession   = "claude --dangerously-skip-permissions"
)

func TestReaper(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		procs    map[int]agentio.Process
		targets  []Target
		onSignal func(*fakeOS, int, syscall.Signal) error
		onRead   func(*fakeOS, int) error
		want     []Outcome
		wantSent []sent
	}{
		{
			name:     "exits on SIGTERM",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: exitOn(syscall.SIGTERM),
			want:     []Outcome{OutcomeStopped},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "ignores SIGTERM: grace then SIGKILL",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: exitOn(syscall.SIGKILL),
			want:     []Outcome{OutcomeForced},
			wantSent: []sent{{10, syscall.SIGTERM}, {10, syscall.SIGKILL}},
		},
		{
			name:     "survives SIGKILL",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: exitOn(),
			want:     []Outcome{OutcomeSurvived},
			wantSent: []sent{{10, syscall.SIGTERM}, {10, syscall.SIGKILL}},
		},
		{
			name:    "pid reused before SIGTERM: never signalled",
			procs:   procs(row(10, 1, t1, aliceSession)),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "pid reused by a non-agent: never signalled",
			procs:   procs(row(10, 1, t0, "/usr/bin/vim notes.md")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "ended before SIGTERM: never signalled",
			procs:   procs(),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "unknown start time: never signalled",
			procs:   procs(row(10, 1, time.Time{}, aliceSession)),
			targets: []Target{{10, time.Time{}}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "unknown start time in the table: never signalled",
			procs:   procs(row(10, 1, time.Time{}, aliceSession)),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded kind at signal time: claude mcp serve",
			procs:   procs(row(10, 1, t0, "claude mcp serve")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded kind at signal time: codex app-server",
			procs:   procs(row(10, 1, t0, "codex app-server")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded kind at signal time: codex mcp-server",
			procs:   procs(row(10, 1, t0, "/opt/homebrew/bin/codex mcp-server")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded kind at signal time: codex mcp serve",
			procs:   procs(row(10, 1, t0, "codex mcp serve")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded kind at signal time: codex mcp list",
			procs:   procs(row(10, 1, t0, "/opt/homebrew/bin/codex mcp list")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded kind at signal time: claude mcp list",
			procs:   procs(row(10, 1, t0, "claude mcp list")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded kind at signal time: codex launcher",
			procs:   procs(row(10, 1, t0, "node /x/@openai/codex/bin/codex.js")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded at signal time: claude shell snapshot",
			procs:   procs(row(10, 1, t0, "/bin/zsh -c source /Users/alice/.claude/shell-snapshots/snapshot-zsh-1.sh 2>/dev/null || true")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:    "excluded at signal time: claude desktop bundled CLI",
			procs:   procs(row(10, 1, t0, "/Users/alice/Library/Application Support/Claude/claude-code/2.1.300/claude --output-format stream-json")),
			targets: []Target{{10, t0}},
			want:    []Outcome{OutcomeChanged},
		},
		{
			name:     "exits during grace and pid is reused before SIGKILL: no SIGKILL",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: func(*fakeOS, int, syscall.Signal) error { return nil }, // ignores SIGTERM
			// Mid-grace the original exits and an unrelated agent session
			// takes the same pid.
			onRead: func(f *fakeOS, n int) error {
				if n == 5 {
					f.procs[10] = row(10, 1, t1, bobSession)
				}
				return nil
			},
			want:     []Outcome{OutcomeStopped},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "pid reused exactly at the pre-SIGKILL check: no SIGKILL",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: exitOn(),
			onRead: func(f *fakeOS, n int) error {
				if n == preKillRead {
					f.procs[10] = row(10, 1, t1, bobSession)
				}
				return nil
			},
			want:     []Outcome{OutcomeStopped},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "pid exec'd into a non-agent before SIGKILL: no SIGKILL",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: exitOn(),
			onRead: func(f *fakeOS, n int) error {
				if n == preKillRead {
					f.procs[10] = row(10, 1, t0, "/usr/bin/vim notes.md") // same pid, same start
				}
				return nil
			},
			want:     []Outcome{OutcomeStopped},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "ended between check and SIGTERM (ESRCH)",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: func(*fakeOS, int, syscall.Signal) error { return syscall.ESRCH },
			want:     []Outcome{OutcomeChanged},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "permission denied",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: func(*fakeOS, int, syscall.Signal) error { return syscall.EPERM },
			want:     []Outcome{OutcomeFailed},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:    "table read fails before SIGTERM: nothing signalled",
			procs:   procs(row(10, 1, t0, aliceSession)),
			targets: []Target{{10, t0}},
			onRead:  func(*fakeOS, int) error { return errors.New("ps: not found") },
			want:    []Outcome{OutcomeFailed},
		},
		{
			name:     "table read fails before SIGKILL: no SIGKILL",
			procs:    procs(row(10, 1, t0, aliceSession)),
			targets:  []Target{{10, t0}},
			onSignal: exitOn(),
			onRead: func(_ *fakeOS, n int) error {
				if n > 1 {
					return errors.New("ps: interrupted")
				}
				return nil
			},
			want:     []Outcome{OutcomeFailed},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name: "mixed batch",
			procs: procs(
				row(10, 1, t0, aliceSession),
				row(11, 1, t1, aliceSession), // reused
				row(12, 1, t0, bobSession),
				row(14, 1, t0, "codex app-server"),
			),
			targets:  []Target{{10, t0}, {11, t0}, {12, t0}, {13, t0}, {14, t0}},
			onSignal: exitOn(syscall.SIGTERM),
			want:     []Outcome{OutcomeStopped, OutcomeChanged, OutcomeStopped, OutcomeChanged, OutcomeChanged},
			wantSent: []sent{{10, syscall.SIGTERM}, {12, syscall.SIGTERM}},
		},
		{
			name: "gemini relaunch pair: the child then its parent",
			procs: procs(
				row(20, 1, t0, "node /opt/homebrew/bin/gemini"),
				row(21, 20, t0, "node --max-old-space-size=8192 /opt/homebrew/bin/gemini --resume latest"),
			),
			targets:  []Target{{21, t0}},
			onSignal: exitOn(syscall.SIGTERM),
			want:     []Outcome{OutcomeStopped},
			wantSent: []sent{{21, syscall.SIGTERM}, {20, syscall.SIGTERM}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeOS{procs: tc.procs, onSignal: tc.onSignal, onRead: tc.onRead}
			got := f.reaper(t).Reap(context.Background(), tc.targets)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d results, want %d", len(got), len(tc.want))
			}
			for i, r := range got {
				if r.Outcome != tc.want[i] {
					t.Errorf("target %d: outcome %q (err %v, reason %q), want %q", r.PID, r.Outcome, r.Err, r.Reason, tc.want[i])
				}
			}
			if !slices.Equal(f.signals, tc.wantSent) {
				t.Fatalf("signals = %v, want %v", f.signals, tc.wantSent)
			}
		})
	}
}

// TestReaperWaitsFullGraceBeforeSIGKILL checks the escalation timing: SIGKILL
// only after the plan's whole grace period has been waited out.
func TestReaperWaitsFullGraceBeforeSIGKILL(t *testing.T) {
	t.Parallel()
	f := &fakeOS{procs: procs(row(10, 1, t0, aliceSession))}
	var sleptAtKill time.Duration
	f.onSignal = func(f *fakeOS, pid int, sig syscall.Signal) error {
		if sig == syscall.SIGKILL {
			sleptAtKill = f.sleptTotal
			delete(f.procs, pid)
		}
		return nil
	}
	got := f.reaper(t).Reap(context.Background(), []Target{{10, t0}})
	if got[0].Outcome != OutcomeForced {
		t.Fatalf("outcome = %q", got[0].Outcome)
	}
	if sleptAtKill < DefaultGrace {
		t.Fatalf("SIGKILL after %s of waiting, want at least the %s grace", sleptAtKill, DefaultGrace)
	}
}

// TestReaperPlanVariants drives the Reaper with plans agentio can return but
// the built-in agents do not on a unix host: a refusal, no graceful signal,
// an interrupt, and a plan that says forcing is unsafe.
func TestReaperPlanVariants(t *testing.T) {
	t.Parallel()
	stopCmd := &agentio.Command{Path: "claude", Args: []string{"stop", aliceConv}}
	cases := []struct {
		name     string
		plan     agentio.StopPlan
		onSignal func(*fakeOS, int, syscall.Signal) error
		want     Outcome
		reason   string
		wantSent []sent
	}{
		{
			name: "refused",
			plan: agentio.StopPlan{Refuse: "an app-server serving many threads"},
			want: OutcomeRefused, reason: "an app-server serving many threads",
		},
		{
			name: "the agent's own stop command",
			plan: agentio.StopPlan{Targets: []int{10}, Graceful: agentio.StopSignalTerminate, Command: stopCmd},
			want: OutcomeRefused, reason: refuseCommand + "claude stop " + aliceConv,
		},
		{
			name: "no targets",
			plan: agentio.StopPlan{Graceful: agentio.StopSignalTerminate},
			want: OutcomeRefused, reason: refuseEmpty,
		},
		{
			name:     "no graceful signal: force at once",
			plan:     agentio.StopPlan{Targets: []int{10}, Graceful: agentio.StopSignalNone, Force: true},
			onSignal: exitOn(syscall.SIGKILL),
			want:     OutcomeForced,
			wantSent: []sent{{10, syscall.SIGKILL}},
		},
		{
			name:     "interrupt",
			plan:     agentio.StopPlan{Targets: []int{10}, Graceful: agentio.StopSignalInterrupt, Grace: time.Second, Force: true},
			onSignal: exitOn(syscall.SIGINT),
			want:     OutcomeStopped,
			wantSent: []sent{{10, syscall.SIGINT}},
		},
		{
			name:     "forcing unsafe: never SIGKILL",
			plan:     agentio.StopPlan{Targets: []int{10}, Graceful: agentio.StopSignalTerminate, Grace: time.Second, Force: false},
			onSignal: exitOn(),
			want:     OutcomeSurvived,
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "a plan pid missing from the table is never signalled",
			plan:     agentio.StopPlan{Targets: []int{10, 99}, Graceful: agentio.StopSignalTerminate, Grace: time.Second, Force: true},
			onSignal: exitOn(syscall.SIGTERM),
			want:     OutcomeStopped,
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeOS{procs: procs(row(10, 1, t0, aliceSession)), onSignal: tc.onSignal}
			r := f.reaper(t)
			r.Plan = func(context.Context, agentio.Process, []agentio.Process) (agentio.StopPlan, bool) {
				return tc.plan, true
			}
			got := r.Reap(context.Background(), []Target{{10, t0}})
			if got[0].Outcome != tc.want || got[0].Reason != tc.reason {
				t.Errorf("outcome %q reason %q (err %v), want %q reason %q", got[0].Outcome, got[0].Reason, got[0].Err, tc.want, tc.reason)
			}
			if !slices.Equal(f.signals, tc.wantSent) {
				t.Fatalf("signals = %v, want %v", f.signals, tc.wantSent)
			}
		})
	}
}

// TestCollectorPlan checks the plans agentop gets from agentio for Claude
// rows on a fixture home: an argv-joined session carries its resume command,
// a background session is refused with `claude stop`, a non-session row has
// no plan.
func TestCollectorPlan(t *testing.T) {
	t.Parallel()
	alice := row(30, 1, t0, "claude --resume "+aliceConv)
	bob := row(31, 1, t0, "claude --dangerously-skip-permissions")
	mcp := row(32, 1, t0, "claude mcp serve")
	fp := &agenttest.FakeProcesses{
		List: []agentio.Process{alice, bob, mcp},
		Cwds: map[int]string{30: "/work/alice", 31: "/work/bob"},
		Envs: map[int][]string{30: {"HOME=/home/alice"}, 31: {"HOME=/home/bob"}},
	}
	c := testCollector(t, fp)
	writePidFile(t, c.Host.UserHome, pidFile{
		PID: 31, SessionID: bobConv, Cwd: "/work/bob", StartedAt: t0.UnixMilli(),
		Kind: string(agentio.SessionBackground), Status: "idle",
	})
	ctx := context.Background()

	plan, ok := c.plan(ctx, alice, fp.List)
	if !ok || plan.Refuse != "" || plan.Resume == nil || !slices.Contains(plan.Resume.Args, aliceConv) {
		t.Fatalf("alice plan = %+v ok=%v, want a stop plan with a resume of %s", plan, ok, aliceConv)
	}
	if !slices.Equal(plan.Targets, []int{30}) || plan.Graceful != agentio.StopSignalTerminate || !plan.Force {
		t.Fatalf("alice plan = %+v, want SIGTERM to 30 then force", plan)
	}

	plan, ok = c.plan(ctx, bob, fp.List)
	if !ok || plan.Command == nil || refusal(plan) == "" || !strings.Contains(refusal(plan), bobConv) {
		t.Fatalf("bob plan = %+v ok=%v, want a refusal naming claude stop %s", plan, ok, bobConv)
	}

	if _, ok := c.plan(ctx, mcp, fp.List); ok {
		t.Fatal("claude mcp serve got a stop plan; it is not a session")
	}
}

// TestResumeLineNonDefaultHome checks the printed resume line of a session
// in a non-default Claude home names that home (CLAUDE_CONFIG_DIR), so it
// resumes where the conversation lives, and never carries another env value.
func TestResumeLineNonDefaultHome(t *testing.T) {
	t.Parallel()
	alice := row(40, 1, t0, "claude --resume "+aliceConv)
	fp := &agenttest.FakeProcesses{List: []agentio.Process{alice}, Cwds: map[int]string{40: "/work/alice"}}
	c := testCollector(t, fp)
	work := filepath.Join(c.Host.UserHome, ".claude-work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	fp.Envs = map[int][]string{40: {"HOME=" + c.Host.UserHome, "CLAUDE_CONFIG_DIR=" + work, "ALICE_TOKEN=never-printed"}}

	plan, ok := c.plan(context.Background(), alice, fp.List)
	if !ok {
		t.Fatal("no plan for alice")
	}
	line := plan.ResumeLine()
	for _, want := range []string{"CLAUDE_CONFIG_DIR=", work, "--resume " + aliceConv} {
		if !strings.Contains(line, want) {
			t.Errorf("resume line %q missing %q", line, want)
		}
	}
	if strings.Contains(line, "never-printed") {
		t.Errorf("resume line %q carries an env value it must not", line)
	}
}

// TestReaperReportsResume checks a stopped session carries the agent's resume
// command, so the user can bring the conversation back.
func TestReaperReportsResume(t *testing.T) {
	t.Parallel()
	alice := row(30, 1, t0, "claude --resume "+aliceConv)
	fp := &agenttest.FakeProcesses{List: []agentio.Process{alice}, Envs: map[int][]string{30: {"HOME=/home/alice"}}}
	c := testCollector(t, fp)
	f := &fakeOS{procs: procs(alice), onSignal: exitOn(syscall.SIGTERM)}
	r := c.Reaper()
	r.Table, r.Alive, r.Signal, r.Poll, r.KillWait = f.table, f.alive, f.signal, testPoll, testKillWait
	r.Sleep = func(d time.Duration) { f.sleptTotal += d }
	r.Now = func() time.Time { return t0.Add(f.sleptTotal) }
	got := r.Reap(context.Background(), []Target{{30, t0}})
	if got[0].Outcome != OutcomeStopped || !strings.Contains(got[0].Resume, "claude --resume "+aliceConv) {
		t.Fatalf("result = %+v, want stopped with a resume of %s", got[0], aliceConv)
	}
}

// TestReaperBackgroundSessionRefused runs the full Reap on a Claude
// background session: agentio's plan says `claude stop`, so nothing is
// signalled.
func TestReaperBackgroundSessionRefused(t *testing.T) {
	t.Parallel()
	bob := row(31, 1, t0, "claude --dangerously-skip-permissions")
	fp := &agenttest.FakeProcesses{List: []agentio.Process{bob}, Envs: map[int][]string{31: {"HOME=/home/bob"}}}
	c := testCollector(t, fp)
	writePidFile(t, c.Host.UserHome, pidFile{PID: 31, SessionID: bobConv, Cwd: "/work/bob", StartedAt: t0.UnixMilli(), Kind: string(agentio.SessionBackground)})
	f := &fakeOS{procs: procs(bob)}
	r := c.Reaper()
	r.Table, r.Alive, r.Signal = f.table, f.alive, f.signal
	got := r.Reap(context.Background(), []Target{{31, t0}})
	if got[0].Outcome != OutcomeRefused || !strings.Contains(got[0].Reason, "stop "+bobConv) {
		t.Fatalf("result = %+v, want refused with claude stop", got[0])
	}
	if len(f.signals) != 0 {
		t.Fatalf("signals = %v, want none", f.signals)
	}
}

func TestReaperEmpty(t *testing.T) {
	t.Parallel()
	f := &fakeOS{}
	if got := f.reaper(t).Reap(context.Background(), nil); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if f.reads != 0 {
		t.Fatalf("empty reap ran %d table reads", f.reads)
	}
}

func TestSummary(t *testing.T) {
	t.Parallel()
	s := Summarize([]Result{
		{Outcome: OutcomeStopped}, {Outcome: OutcomeForced}, {Outcome: OutcomeChanged},
		{Outcome: OutcomeRefused}, {Outcome: OutcomeSurvived}, {Outcome: OutcomeFailed},
	})
	want := "reaped 2 (1 after SIGKILL), 1 skipped: process changed or ended, 1 refused by the agent's stop plan, 1 survived, 1 failed"
	if got := s.String(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := Summarize([]Result{{Outcome: OutcomeStopped}}).String(); got != "reaped 1" {
		t.Fatalf("got %q", got)
	}
	if !(Result{Outcome: OutcomeChanged}).Gone() || (Result{Outcome: OutcomeRefused}).Gone() {
		t.Fatal("Gone: changed is gone, refused is not")
	}
}

// TestReaperGracePollExactStart checks the grace poll matches start times
// exactly: a pid reused one second after the original started (inside
// agentio's default 2s tolerance) counts as ended at once, so the reaper stops
// polling and never reaches the SIGKILL step.
func TestReaperGracePollExactStart(t *testing.T) {
	t.Parallel()
	f := &fakeOS{procs: procs(row(10, 1, t0, aliceSession))}
	f.onSignal = func(*fakeOS, int, syscall.Signal) error { return nil } // ignores SIGTERM
	f.onRead = func(f *fakeOS, n int) error {
		if n == 2 {
			f.procs[10] = row(10, 1, t0.Add(time.Second), bobSession)
		}
		return nil
	}
	got := f.reaper(t).Reap(context.Background(), []Target{{10, t0}})
	if got[0].Outcome != OutcomeStopped {
		t.Fatalf("outcome %q, want stopped", got[0].Outcome)
	}
	if !slices.Equal(f.signals, []sent{{10, syscall.SIGTERM}}) {
		t.Fatalf("signals = %v, want only the SIGTERM", f.signals)
	}
	if f.reads != 2 {
		t.Fatalf("table reads = %d, want 2 (the pre-signal check and one poll)", f.reads)
	}
}
