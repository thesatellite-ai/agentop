package collect

import (
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/thesatellite-ai/agentop/internal/discover"
)

// fakeOS is a scripted process table plus a signal recorder. No test here
// touches a real process: Signal only records and mutates the fake table.
type fakeOS struct {
	t       *testing.T
	procs   map[int]discover.Proc
	signals []sent
	// onSignal lets a case decide what a signal does to the fake table.
	onSignal func(f *fakeOS, pid int, sig syscall.Signal) error
	// onLookup runs before each lookup (lookup count starts at 1); it can
	// end, replace or fail processes over time.
	onLookup   func(f *fakeOS, n int) error
	lookups    int
	sleptTotal time.Duration
}

type sent struct {
	pid int
	sig syscall.Signal
}

func (f *fakeOS) lookup(pids []int) (map[int]discover.Proc, error) {
	f.lookups++
	if f.onLookup != nil {
		if err := f.onLookup(f, f.lookups); err != nil {
			return nil, err
		}
	}
	out := map[int]discover.Proc{}
	for _, pid := range pids {
		if p, ok := f.procs[pid]; ok {
			out[pid] = p
		}
	}
	return out, nil
}

func (f *fakeOS) signal(pid int, sig syscall.Signal) error {
	f.signals = append(f.signals, sent{pid, sig})
	if f.onSignal != nil {
		return f.onSignal(f, pid, sig)
	}
	return nil
}

func (f *fakeOS) reaper() Reaper {
	return Reaper{
		Lookup:   f.lookup,
		Signal:   f.signal,
		Sleep:    func(d time.Duration) { f.sleptTotal += d },
		Grace:    time.Second,
		Poll:     100 * time.Millisecond,
		KillWait: 300 * time.Millisecond,
	}
}

var (
	t0      = time.Date(2026, 11, 1, 10, 0, 0, 0, time.Local)
	t1      = t0.Add(time.Hour)
	session = "claude --resume abc"
)

func proc(pid int, start time.Time, cmd string) discover.Proc {
	return discover.Proc{PID: pid, PPID: 1, StartedAt: start, Command: cmd}
}

// exitOn makes the fake process exit when it receives one of sigs.
func exitOn(sigs ...syscall.Signal) func(*fakeOS, int, syscall.Signal) error {
	return func(f *fakeOS, pid int, sig syscall.Signal) error {
		if _, ok := f.procs[pid]; !ok {
			return syscall.ESRCH
		}
		for _, s := range sigs {
			if s == sig {
				delete(f.procs, pid)
			}
		}
		return nil
	}
}

func TestReaper(t *testing.T) {
	cases := []struct {
		name     string
		procs    map[int]discover.Proc
		targets  []Target
		onSignal func(*fakeOS, int, syscall.Signal) error
		onLookup func(*fakeOS, int) error
		want     []Outcome
		wantSent []sent
	}{
		{
			name:     "exits on SIGTERM",
			procs:    map[int]discover.Proc{10: proc(10, t0, session)},
			targets:  []Target{{10, t0}},
			onSignal: exitOn(syscall.SIGTERM),
			want:     []Outcome{OutcomeStopped},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "ignores SIGTERM: grace then SIGKILL",
			procs:    map[int]discover.Proc{10: proc(10, t0, session)},
			targets:  []Target{{10, t0}},
			onSignal: exitOn(syscall.SIGKILL),
			want:     []Outcome{OutcomeForced},
			wantSent: []sent{{10, syscall.SIGTERM}, {10, syscall.SIGKILL}},
		},
		{
			name:     "survives SIGKILL",
			procs:    map[int]discover.Proc{10: proc(10, t0, session)},
			targets:  []Target{{10, t0}},
			onSignal: exitOn(),
			want:     []Outcome{OutcomeSurvived},
			wantSent: []sent{{10, syscall.SIGTERM}, {10, syscall.SIGKILL}},
		},
		{
			name:     "pid reused before SIGTERM: never signalled",
			procs:    map[int]discover.Proc{10: proc(10, t1, session)},
			targets:  []Target{{10, t0}},
			want:     []Outcome{OutcomeChanged},
			wantSent: nil,
		},
		{
			name:     "pid reused by a non-agent: never signalled",
			procs:    map[int]discover.Proc{10: proc(10, t0, "/usr/bin/vim notes.md")},
			targets:  []Target{{10, t0}},
			want:     []Outcome{OutcomeChanged},
			wantSent: nil,
		},
		{
			name:     "ended before SIGTERM: never signalled",
			procs:    map[int]discover.Proc{},
			targets:  []Target{{10, t0}},
			want:     []Outcome{OutcomeChanged},
			wantSent: nil,
		},
		{
			name:     "unknown start time: never signalled",
			procs:    map[int]discover.Proc{10: proc(10, time.Time{}, session)},
			targets:  []Target{{10, time.Time{}}},
			want:     []Outcome{OutcomeChanged},
			wantSent: nil,
		},
		{
			name:     "excluded kind at signal time: claude mcp serve",
			procs:    map[int]discover.Proc{10: proc(10, t0, "claude mcp serve")},
			targets:  []Target{{10, t0}},
			want:     []Outcome{OutcomeChanged},
			wantSent: nil,
		},
		{
			name:     "excluded kind at signal time: codex app-server",
			procs:    map[int]discover.Proc{10: proc(10, t0, "codex app-server")},
			targets:  []Target{{10, t0}},
			want:     []Outcome{OutcomeChanged},
			wantSent: nil,
		},
		{
			name:     "excluded kind at signal time: codex launcher",
			procs:    map[int]discover.Proc{10: proc(10, t0, "node /x/@openai/codex/bin/codex.js")},
			targets:  []Target{{10, t0}},
			want:     []Outcome{OutcomeChanged},
			wantSent: nil,
		},
		{
			name:    "exits during grace and pid is reused before SIGKILL: no SIGKILL",
			procs:   map[int]discover.Proc{10: proc(10, t0, session)},
			targets: []Target{{10, t0}},
			onSignal: func(f *fakeOS, pid int, sig syscall.Signal) error {
				return nil // ignores SIGTERM for now
			},
			// After the SIGTERM poll window the original exits and an
			// unrelated agent session takes the same pid.
			onLookup: func(f *fakeOS, n int) error {
				if n == 5 {
					f.procs[10] = proc(10, t1, session)
				}
				return nil
			},
			want:     []Outcome{OutcomeStopped},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "pid reused exactly at the pre-SIGKILL check: no SIGKILL",
			procs:    map[int]discover.Proc{10: proc(10, t0, session)},
			targets:  []Target{{10, t0}},
			onSignal: exitOn(),
			// Lookup 1 is the pre-SIGTERM check, 2..12 the grace polls
			// (0..1s every 100ms), 13 the pre-SIGKILL check.
			onLookup: func(f *fakeOS, n int) error {
				if n == 13 {
					f.procs[10] = proc(10, t1, session)
				}
				return nil
			},
			want:     []Outcome{OutcomeStopped},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "ended between check and SIGTERM (ESRCH)",
			procs:    map[int]discover.Proc{10: proc(10, t0, session)},
			targets:  []Target{{10, t0}},
			onSignal: func(*fakeOS, int, syscall.Signal) error { return syscall.ESRCH },
			want:     []Outcome{OutcomeChanged},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "permission denied",
			procs:    map[int]discover.Proc{10: proc(10, t0, session)},
			targets:  []Target{{10, t0}},
			onSignal: func(*fakeOS, int, syscall.Signal) error { return syscall.EPERM },
			want:     []Outcome{OutcomeFailed},
			wantSent: []sent{{10, syscall.SIGTERM}},
		},
		{
			name:     "lookup fails before SIGTERM: nothing signalled",
			procs:    map[int]discover.Proc{10: proc(10, t0, session)},
			targets:  []Target{{10, t0}},
			onLookup: func(*fakeOS, int) error { return errors.New("ps: not found") },
			want:     []Outcome{OutcomeFailed},
			wantSent: nil,
		},
		{
			name:     "lookup fails before SIGKILL: no SIGKILL",
			procs:    map[int]discover.Proc{10: proc(10, t0, session)},
			targets:  []Target{{10, t0}},
			onSignal: exitOn(),
			onLookup: func(_ *fakeOS, n int) error {
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
			procs: map[int]discover.Proc{
				10: proc(10, t0, session),
				11: proc(11, t1, session), // reused
				12: proc(12, t0, session),
			},
			targets:  []Target{{10, t0}, {11, t0}, {12, t0}, {13, t0}},
			onSignal: exitOn(syscall.SIGTERM),
			want:     []Outcome{OutcomeStopped, OutcomeChanged, OutcomeStopped, OutcomeChanged},
			wantSent: []sent{{10, syscall.SIGTERM}, {12, syscall.SIGTERM}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeOS{t: t, procs: tc.procs, onSignal: tc.onSignal, onLookup: tc.onLookup}
			got := f.reaper().Reap(tc.targets)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d results, want %d", len(got), len(tc.want))
			}
			for i, r := range got {
				if r.Outcome != tc.want[i] {
					t.Errorf("target %d: outcome %q (err %v), want %q", r.PID, r.Outcome, r.Err, tc.want[i])
				}
			}
			if len(f.signals) != len(tc.wantSent) {
				t.Fatalf("signals = %v, want %v", f.signals, tc.wantSent)
			}
			for i := range f.signals {
				if f.signals[i] != tc.wantSent[i] {
					t.Fatalf("signals = %v, want %v", f.signals, tc.wantSent)
				}
			}
		})
	}
}

// TestReaperWaitsFullGraceBeforeSIGKILL checks the escalation timing: SIGKILL
// only after the whole grace period has been waited out.
func TestReaperWaitsFullGraceBeforeSIGKILL(t *testing.T) {
	f := &fakeOS{t: t, procs: map[int]discover.Proc{10: proc(10, t0, session)}}
	var sleptAtKill time.Duration
	f.onSignal = func(f *fakeOS, pid int, sig syscall.Signal) error {
		if sig == syscall.SIGKILL {
			sleptAtKill = f.sleptTotal
			delete(f.procs, pid)
		}
		return nil
	}
	r := f.reaper()
	got := r.Reap([]Target{{10, t0}})
	if got[0].Outcome != OutcomeForced {
		t.Fatalf("outcome = %q", got[0].Outcome)
	}
	if sleptAtKill < r.Grace {
		t.Fatalf("SIGKILL after %s of waiting, want at least the %s grace", sleptAtKill, r.Grace)
	}
}

func TestReaperEmpty(t *testing.T) {
	f := &fakeOS{t: t}
	if got := f.reaper().Reap(nil); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if f.lookups != 0 {
		t.Fatalf("empty reap ran %d lookups", f.lookups)
	}
}

func TestSummary(t *testing.T) {
	s := Summarize([]Result{
		{Outcome: OutcomeStopped}, {Outcome: OutcomeForced}, {Outcome: OutcomeChanged},
		{Outcome: OutcomeSurvived}, {Outcome: OutcomeFailed},
	})
	want := "reaped 2 (1 after SIGKILL), 1 skipped: process changed or ended, 1 survived SIGKILL, 1 failed"
	if got := s.String(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := Summarize([]Result{{Outcome: OutcomeStopped}}).String(); got != "reaped 1" {
		t.Fatalf("got %q", got)
	}
}
