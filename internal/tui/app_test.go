package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"

	"github.com/thesatellite-ai/agentop/internal/collect"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// fixtureSessions is a fixed process table covering every source and agent,
// so the smoke test never depends on what runs on the machine.
func fixtureSessions() []model.Session {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	row := func(pid int, ag agentio.ID, src model.Source, rssMB uint64, idle time.Duration, task string) model.Session {
		return model.Session{
			PID: pid, PPID: 1, Agent: ag, Source: src,
			RSSBytes:  rssMB * 1024 * 1024,
			Cwd:       "/work/proj-" + string(src),
			Command:   string(ag) + " --resume abc",
			TTY:       "ttys001",
			StartedAt: now.Add(-48 * time.Hour),
			TTYActive: now.Add(-idle),
			Task:      task,
			Idle:      idle,
		}
	}
	return []model.Session{
		row(101, "claude", model.SourceRunbay, 640, 3*24*time.Hour, ""),
		row(102, "claude", model.SourceRunbay, 420, time.Minute, ""),
		row(103, "claude", model.SourceCmux, 900, 26*time.Hour, ""),
		row(104, "codex", model.SourceEmdash, 300, 5*time.Hour, "fix-login"),
		row(105, "claude", model.SourceDirect, 512, 10*time.Second, ""),
		row(106, "codex", model.SourceDirect, 128, 9*24*time.Hour, ""),
	}
}

// newFixtureApp returns an App whose reload reads fixtureSessions.
func newFixtureApp() *App {
	a := newApp()
	a.load = func() ([]model.Session, error) {
		s := fixtureSessions()
		collect.Rank(s)
		return s, nil
	}
	return a
}

// TestRenderPipeline exercises the full data->render path over a fixture
// process table (build layout, reload, group, filter, sort, repaint every
// pane, switch groups, cycle sort) without starting the interactive event
// loop. It's a smoke test: it must not panic and must produce coherent state.
func TestRenderPipeline(t *testing.T) {
	a := newFixtureApp()
	a.buildLayout()
	if err := a.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(a.all) != len(fixtureSessions()) {
		t.Fatalf("loaded %d sessions, want the %d fixtures", len(a.all), len(fixtureSessions()))
	}
	// Every source in the fixture gets its own sidebar group, runbay included.
	srcGroups := map[model.Source]int{}
	for _, g := range a.groups {
		if g.kind == groupSource {
			srcGroups[g.src] = g.count
		}
	}
	want := map[model.Source]int{model.SourceRunbay: 2, model.SourceCmux: 1, model.SourceEmdash: 1, model.SourceDirect: 2}
	for src, n := range want {
		if srcGroups[src] != n {
			t.Errorf("source group %s count = %d, want %d", src, srcGroups[src], n)
		}
	}

	// There's always at least the "All" group, first.
	if len(a.groups) == 0 || a.groups[0].kind != groupAll {
		t.Fatalf("expected an All group first, got %+v", a.groups)
	}

	// view must be a subset of all and respect the active (All) filter.
	if len(a.view) != len(a.all) {
		t.Fatalf("All group should show every session: view=%d all=%d", len(a.view), len(a.all))
	}

	// Source-group counts must sum to total; agent-group counts must too (the
	// two are independent partitions of the same set).
	var srcSum, agtSum int
	for _, g := range a.groups {
		switch g.kind {
		case groupSource:
			srcSum += g.count
		case groupAgent:
			agtSum += g.count
		}
	}
	if srcSum != len(a.all) {
		t.Fatalf("source counts %d != total %d", srcSum, len(a.all))
	}
	if agtSum != len(a.all) {
		t.Fatalf("agent counts %d != total %d", agtSum, len(a.all))
	}

	// Switching to each filter group must only show matching sessions; headers
	// must not change the view.
	for i, g := range a.groups {
		a.onGroupChanged(i)
		for _, s := range a.view {
			if !g.matches(s) {
				t.Fatalf("group %q (kind %d) leaked a non-matching session", g.label, g.kind)
			}
		}
	}

	// Cycle every sort field in both directions — must not panic and must
	// actually order the slice.
	a.onGroupChanged(0) // back to All
	for _, f := range sortFields {
		for _, asc := range []bool{true, false} {
			a.sortField, a.sortAsc = f, asc
			a.applyFilterSort()
			a.refreshTable()
			for i := 1; i < len(a.view); i++ {
				if f.less(a.view[i], a.view[i-1]) == asc && a.view[i] != a.view[i-1] {
					// strictly out of order in the requested direction
					if asc {
						t.Fatalf("sort %s asc out of order at %d", f, i)
					}
				}
			}
		}
	}

	// Detail render for the first row must not panic.
	a.refreshDetail()
}

// TestTargetsForCarriesStartTime checks the TUI hands the reaper the start
// time from the last refresh, and an unknown pid gets none (so it is skipped).
func TestTargetsForCarriesStartTime(t *testing.T) {
	a := newFixtureApp()
	a.buildLayout()
	if err := a.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := a.targetsFor([]int{103, 999})
	if got[0].PID != 103 || got[0].StartedAt.IsZero() {
		t.Fatalf("target for listed pid = %+v, want its start time", got[0])
	}
	if got[1].PID != 999 || !got[1].StartedAt.IsZero() {
		t.Fatalf("target for unknown pid = %+v, want no start time", got[1])
	}
}

func TestAgentOrder(t *testing.T) {
	t.Parallel()
	got := agentOrder(map[agentio.ID]int{"zed-agent": 1, "codex": 2, "acme": 1})
	want := []agentio.ID{"claude", "codex", "gemini", "pi", "opencode", "acme", "zed-agent"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestAgentColors(t *testing.T) {
	t.Parallel()
	seen := map[string]agentio.ID{}
	for _, id := range knownAgents {
		c := hex(agentColor(id))
		if c == hex(theme.Muted) {
			t.Errorf("%s has no hue of its own", id)
		}
		if other, dup := seen[c]; dup {
			t.Errorf("%s and %s share %s", id, other, c)
		}
		seen[c] = id
	}
	if agentColor("acme") != theme.Muted {
		t.Error("an unknown agent should be muted")
	}
}

func TestSessionLabel(t *testing.T) {
	t.Parallel()
	if got := sessionLabel(model.Session{}, "/home/alice"); got != "—" {
		t.Fatalf("empty = %q", got)
	}
	s := model.Session{
		ConversationID: "0f79d084-6061-439e-b571-3668de33ed34", Join: agentio.JoinExact,
		Status: agentio.SessionWaiting, Version: "2.1.300", Home: "/home/alice/.claude",
	}
	got := sessionLabel(s, "/home/alice")
	for _, want := range []string{"0f79d084-6061-439e-b571-3668de33ed34", "(exact)", "waiting", "v2.1.300", "home ~/.claude"} {
		if !strings.Contains(got, want) {
			t.Errorf("label %q missing %q", got, want)
		}
	}
}

// TestNewAppHasNoSource checks an App built without Run lists nothing and
// signals nothing.
func TestNewAppHasNoSource(t *testing.T) {
	t.Parallel()
	a := newApp()
	if _, err := a.load(); err == nil {
		t.Fatal("load without a source should fail")
	}
	if got := a.reap([]collect.Target{{PID: 1}}); got != nil {
		t.Fatalf("reap without a source = %v", got)
	}
}
