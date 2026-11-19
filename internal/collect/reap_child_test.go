package collect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/ubgo/agentio"
)

// linkAs links dir/name to the executable src, so a child this test spawns
// through the link has argv[0] basename name: agentio classifies a basename
// "claude" as a Claude CLI session. A link, not a copy: macOS refuses to run
// a copied platform binary.
func linkAs(t *testing.T, src, dir, name string) string {
	t.Helper()
	dst := filepath.Join(dir, name)
	if err := os.Symlink(src, dst); err != nil {
		t.Skipf("cannot link %s: %v", src, err)
	}
	return dst
}

// spawnSession starts this test's own child as a fake "claude" session and
// returns it with the start time the real process table reads for it.
func spawnSession(t *testing.T, c *Collector, src string, args ...string) (*exec.Cmd, time.Time) {
	t.Helper()
	bin := linkAs(t, src, t.TempDir(), "claude")
	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start a child: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }() // reap the zombie so the pid leaves the table
	t.Cleanup(func() {
		_ = cmd.Process.Kill() // our own child only; fails harmlessly once it exited
		<-done
	})
	table, err := c.table(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range table {
		if p.PID == cmd.Process.Pid {
			info, ok := agentio.ClassifyProcess(c.Agents, p)
			if !ok || !info.Session || p.Start.IsZero() {
				t.Skipf("child row %+v is not classified as a session here (%+v %v)", p, info, ok)
			}
			return cmd, p.Start
		}
	}
	t.Fatalf("child %d not in the process table", cmd.Process.Pid)
	return nil, time.Time{}
}

// TestReapOwnChild runs the production Reaper (real process table, real
// kill(2), agentio's real plan) against children this test spawned: one
// exits on SIGTERM, one ignores SIGTERM and needs SIGKILL after the grace,
// and a stale start time is never signalled. No other process is touched.
func TestReapOwnChild(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes and waits out a real grace period")
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("exits on SIGTERM", func(t *testing.T) {
		cmd, start := spawnSession(t, c, "/bin/sleep", "60")
		got := c.Reaper().Reap(ctx, []Target{{cmd.Process.Pid, start}})
		if got[0].Outcome != OutcomeStopped {
			t.Fatalf("outcome %q (err %v, reason %q), want stopped", got[0].Outcome, got[0].Err, got[0].Reason)
		}
	})

	t.Run("stale start time is never signalled", func(t *testing.T) {
		cmd, start := spawnSession(t, c, "/bin/sleep", "60")
		got := c.Reaper().Reap(ctx, []Target{{cmd.Process.Pid, start.Add(-time.Hour)}})
		if got[0].Outcome != OutcomeChanged {
			t.Fatalf("outcome %q, want changed", got[0].Outcome)
		}
		if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
			t.Fatalf("the child is gone (%v); it must not have been signalled", err)
		}
	})

	t.Run("ignores SIGTERM: SIGKILL after the grace", func(t *testing.T) {
		cmd, start := spawnSession(t, c, "/bin/sh", "-c", "trap '' TERM; while :; do sleep 0.1; done")
		began := time.Now()
		got := c.Reaper().Reap(ctx, []Target{{cmd.Process.Pid, start}})
		if got[0].Outcome != OutcomeForced {
			t.Fatalf("outcome %q (err %v), want killed after grace", got[0].Outcome, got[0].Err)
		}
		if waited := time.Since(began); waited < DefaultGrace {
			t.Fatalf("forced after %s, want at least the %s grace", waited, DefaultGrace)
		}
	})
}
