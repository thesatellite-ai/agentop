package tui

import (
	"testing"
	"time"

	"github.com/thesatellite-ai/agentop/internal/model"
)

func sample() []model.Session {
	return []model.Session{
		{PID: 3, Agent: model.AgentCodex, Source: model.SourceDirect, Task: "z", Cwd: "/c", RSSBytes: 30, Idle: 3 * time.Hour},
		{PID: 1, Agent: model.AgentClaude, Source: model.SourceCmux, Task: "a", Cwd: "/a", RSSBytes: 10, Idle: time.Hour},
		{PID: 2, Agent: model.AgentClaude, Source: model.SourceEmdash, Task: "m", Cwd: "/b", RSSBytes: 20, Idle: 2 * time.Hour},
	}
}

// firstPID returns the head PID after sorting by field+dir.
func firstPID(f SortField, asc bool) int {
	s := sample()
	sortSessions(s, f, asc)
	return s[0].PID
}

func TestSortSessions(t *testing.T) {
	cases := []struct {
		field   SortField
		asc     bool
		wantPID int
	}{
		{SortIdle, false, 3}, // most idle first
		{SortIdle, true, 1},  // least idle first
		{SortMem, false, 3},  // largest mem
		{SortMem, true, 1},
		{SortPID, true, 1},
		{SortPID, false, 3},
		{SortTask, true, 1},  // "a" first
		{SortTask, false, 3}, // "z" first
		{SortCwd, true, 1},   // /a first
		{SortAgent, true, 1}, // claude < codex
		{SortAgent, false, 3},
		{SortSource, true, 1}, // cmux < direct < emdash
	}
	for _, tc := range cases {
		if got := firstPID(tc.field, tc.asc); got != tc.wantPID {
			t.Errorf("sort %s asc=%v: head pid = %d, want %d", tc.field, tc.asc, got, tc.wantPID)
		}
	}
}

func TestSortFieldString(t *testing.T) {
	want := map[SortField]string{
		SortIdle: "idle", SortMem: "memory", SortPID: "pid",
		SortAgent: "agent", SortSource: "source", SortTask: "task", SortCwd: "cwd",
	}
	for f, s := range want {
		if f.String() != s {
			t.Errorf("%d.String() = %q, want %q", f, f.String(), s)
		}
	}
}
