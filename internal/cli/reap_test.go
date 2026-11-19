package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/thesatellite-ai/agentop/internal/collect"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// aliceConv is the conversation alice's session resumes.
const aliceConv = "0f79d084-6061-439e-b571-3668de33ed34"

// row builds a darwin-shaped process row (Argv split on spaces).
func row(pid int, start time.Time, command string) agentio.Process {
	argv := strings.Fields(command)
	return agentio.Process{PID: pid, PPID: 1, Start: start, Exe: argv[0], Argv: argv, CommandLine: command}
}

// testReaper is the production Reaper (agentio classification and stop
// plans on a fixture host) on a fake process table: signals only mutate the
// table, no real process is touched.
func testReaper(t *testing.T, table map[int]agentio.Process, signal collect.Signaler) collect.Reaper {
	t.Helper()
	var list []agentio.Process
	for _, p := range table {
		list = append(list, p)
	}
	fp := &agenttest.FakeProcesses{List: list, Envs: map[int][]string{}}
	c, err := collect.NewWithHost(agenttest.NewHost(t, agenttest.WithProcesses(fp)))
	if err != nil {
		t.Fatal(err)
	}
	r := c.Reaper()
	r.Table = func(context.Context) ([]agentio.Process, error) {
		out := make([]agentio.Process, 0, len(table))
		for _, p := range table {
			out = append(out, p)
		}
		return out, nil
	}
	host := agentio.Host{Processes: tableLister(r.Table)}
	r.Alive = func(ctx context.Context, refs []agentio.ProcessRef) ([]agentio.Liveness, error) {
		return host.ProcessesAlive(ctx, refs, agentio.ExactStart())
	}
	r.Signal = signal
	var slept time.Duration
	r.Sleep = func(d time.Duration) { slept += d }
	r.Now = func() time.Time { return time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC).Add(slept) }
	r.Poll = 100 * time.Millisecond
	r.KillWait = 200 * time.Millisecond
	return r
}

// tableLister serves a TableReader as an agentio.ProcessLister, so agentio's
// batch liveness check reads the same fake table.
type tableLister collect.TableReader

func (l tableLister) Processes(ctx context.Context) ([]agentio.Process, error) { return l(ctx) }

func (l tableLister) Cwd(context.Context, int) (string, error) { return "", errors.New("no cwd") }

// TestReapSessionsUsesGraceThenKill runs the CLI reap path on a fake process
// table: one session exits on SIGTERM, one ignores it and needs SIGKILL, one
// pid was reused since the list. No real process is signalled.
func TestReapSessionsUsesGraceThenKill(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	table := map[int]agentio.Process{
		1: row(1, start, "claude --resume "+aliceConv),
		2: row(2, start, "claude --resume b"),
		3: row(3, start.Add(time.Hour), "claude --resume c"), // reused
	}
	var sent []string
	r := testReaper(t, table, func(pid int, sig syscall.Signal) error {
		sent = append(sent, sig.String())
		if (pid == 1 && sig == syscall.SIGTERM) || sig == syscall.SIGKILL {
			delete(table, pid)
		}
		return nil
	})
	victims := []model.Session{
		{PID: 1, StartedAt: start}, {PID: 2, StartedAt: start}, {PID: 3, StartedAt: start},
	}
	var out bytes.Buffer
	if err := reapSessions(context.Background(), &out, victims, r); err != nil {
		t.Fatalf("reapSessions: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"pid 1: stopped; resume: claude --resume " + aliceConv,
		"pid 2: killed after grace",
		"pid 3: process changed or ended",
		"Reaped 2 (1 after SIGKILL), 1 skipped: process changed or ended.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Join(sent, ",") != "terminated,terminated,killed" {
		t.Errorf("signals = %v, want SIGTERM to pids 1 and 2, then SIGKILL to 2", sent)
	}
}

func TestReapSessionsReportsFailure(t *testing.T) {
	t.Parallel()
	r := testReaper(t, map[int]agentio.Process{}, func(int, syscall.Signal) error {
		t.Error("must not signal when identity cannot be confirmed")
		return nil
	})
	r.Table = func(context.Context) ([]agentio.Process, error) { return nil, errors.New("ps failed") }
	var out bytes.Buffer
	err := reapSessions(context.Background(), &out, []model.Session{{PID: 7, StartedAt: time.Now()}}, r)
	if err == nil || !strings.Contains(out.String(), "pid 7: failed") {
		t.Fatalf("err = %v, output:\n%s", err, out.String())
	}
}

func TestResultLine(t *testing.T) {
	t.Parallel()
	resume := "cd /work/alice && CLAUDE_CONFIG_DIR=/home/alice/.claude-work claude --resume " + aliceConv
	for _, tc := range []struct {
		name string
		res  collect.Result
		want string
	}{
		{"clean stop", collect.Result{Target: collect.Target{PID: 1}, Outcome: collect.OutcomeStopped}, ""},
		{"stop with resume", collect.Result{Target: collect.Target{PID: 1}, Outcome: collect.OutcomeStopped, Resume: resume},
			"pid 1: stopped; resume: " + resume},
		{"refused", collect.Result{Target: collect.Target{PID: 2}, Outcome: collect.OutcomeRefused, Reason: "stop it with the agent's command: claude stop x"},
			"pid 2: refused (stop it with the agent's command: claude stop x)"},
		{"failed", collect.Result{Target: collect.Target{PID: 3}, Outcome: collect.OutcomeFailed, Err: syscall.EPERM},
			"pid 3: failed (operation not permitted)"},
	} {
		if got := resultLine(tc.res); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
