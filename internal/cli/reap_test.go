package cli

import (
	"bytes"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/thesatellite-ai/agentop/internal/collect"
	"github.com/thesatellite-ai/agentop/internal/discover"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// TestReapSessionsUsesGraceThenKill runs the CLI reap path on a fake process
// table: one session exits on SIGTERM, one ignores it and needs SIGKILL, one
// pid was reused since the list. No real process is signalled.
func TestReapSessionsUsesGraceThenKill(t *testing.T) {
	start := time.Date(2026, 11, 1, 9, 0, 0, 0, time.Local)
	table := map[int]discover.Proc{
		1: {PID: 1, StartedAt: start, Command: "claude --resume a"},
		2: {PID: 2, StartedAt: start, Command: "claude --resume b"},
		3: {PID: 3, StartedAt: start.Add(time.Hour), Command: "claude --resume c"}, // reused
	}
	var sent []string
	r := collect.Reaper{
		Lookup: func(pids []int) (map[int]discover.Proc, error) {
			out := map[int]discover.Proc{}
			for _, p := range pids {
				if row, ok := table[p]; ok {
					out[p] = row
				}
			}
			return out, nil
		},
		Signal: func(pid int, sig syscall.Signal) error {
			sent = append(sent, sig.String())
			if (pid == 1 && sig == syscall.SIGTERM) || sig == syscall.SIGKILL {
				delete(table, pid)
			}
			return nil
		},
		Sleep:    func(time.Duration) {},
		Grace:    time.Second,
		Poll:     100 * time.Millisecond,
		KillWait: 200 * time.Millisecond,
	}
	victims := []model.Session{
		{PID: 1, StartedAt: start}, {PID: 2, StartedAt: start}, {PID: 3, StartedAt: start},
	}
	var out bytes.Buffer
	if err := reapSessions(&out, victims, r); err != nil {
		t.Fatalf("reapSessions: %v", err)
	}
	got := out.String()
	for _, want := range []string{
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
	r := collect.Reaper{
		Lookup: func([]int) (map[int]discover.Proc, error) {
			return map[int]discover.Proc{}, syscall.ENOENT
		},
		Signal: func(int, syscall.Signal) error {
			t.Fatal("must not signal when identity cannot be confirmed")
			return nil
		},
		Sleep: func(time.Duration) {},
	}
	var out bytes.Buffer
	err := reapSessions(&out, []model.Session{{PID: 7, StartedAt: time.Now()}}, r)
	if err == nil || !strings.Contains(out.String(), "pid 7: failed") {
		t.Fatalf("err = %v, output:\n%s", err, out.String())
	}
}
