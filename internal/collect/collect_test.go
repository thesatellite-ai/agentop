package collect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/thesatellite-ai/agentop/internal/emdash"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// fixtureTable is a process table shaped like a real Mac with every launcher
// and every row agentop must not list (users and paths are made up).
func fixtureTable() []agentio.Process {
	return []agentio.Process{
		row(1, 0, t0, "/sbin/launchd"),
		// alice: a Claude session in a runbay pty, resumed by id.
		row(100, 1, t0, "/Users/alice/.runbay/bin/runbay-ptyd-0.1.0 ptyd --daemon"),
		row(101, 100, t0, "/bin/zsh -l"),
		row(102, 101, t0, "claude --dangerously-skip-permissions --resume "+aliceConv),
		// bob: a Claude session in cmux, busy per its pid file.
		row(110, 1, t0, "/Applications/cmux.app/Contents/MacOS/cmux"),
		row(111, 110, t0, "/bin/zsh /T/cmux-agent-resume/claude-abc.zsh"),
		row(112, 111, t1, "claude"),
		// carol: Codex through npm in an emdash worktree; only the native
		// child is a session, the node launcher is not.
		row(120, 1, t0, "/Applications/Emdash.app/Contents/MacOS/Emdash"),
		row(121, 120, t0, "node /opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js"),
		row(122, 121, t0, "/opt/homebrew/lib/node_modules/@openai/codex/node_modules/@openai/codex-darwin-arm64/vendor/aarch64-apple-darwin/codex/codex"),
		// Gemini and pi in a plain terminal.
		row(130, 1, t0, "-zsh"),
		row(131, 130, t0, "node --max-old-space-size=8192 /opt/homebrew/bin/gemini --resume latest"),
		row(132, 130, t0, "pi"),
		// Never listed: servers, launchers, desktop helpers, tools.
		row(200, 1, t0, "claude mcp serve"),
		row(201, 1, t0, "codex app-server --listen unix://x"),
		row(202, 1, t0, "/opt/homebrew/bin/codex mcp-server"),
		row(210, 1, t0, "codex mcp serve"),
		row(211, 1, t0, "/opt/homebrew/bin/codex mcp list"),
		row(212, 1, t0, "claude mcp list"),
		row(213, 1, t0, "/Users/alice/.local/bin/claude mcp add bob-tools -- npx bob-tools"),
		row(203, 1, t0, "/Applications/Claude.app/Contents/MacOS/Claude"),
		row(204, 1, t0, "/x/claude-in-chrome/server"),
		row(205, 1, t0, "/Users/alice/Library/Application Support/Claude/claude-code/2.1.300/claude --output-format stream-json"),
		row(206, 1, t0, "/bin/zsh -c source /Users/alice/.claude/shell-snapshots/snapshot-zsh-1.sh 2>/dev/null || true"),
		row(207, 1, t0, "node /opt/homebrew/bin/npm install -g @anthropic-ai/claude-code"),
		row(208, 1, t0, "/usr/bin/vim main.go"),
		row(209, 1, t0, "/usr/bin/pip install x"),
	}
}

func TestSessions(t *testing.T) {
	t.Parallel()
	table := fixtureTable()
	table[3].RSS, table[3].TTY = 640<<20, "ttys001"
	fp := &agenttest.FakeProcesses{
		List: table,
		Cwds: map[int]string{102: "/work/alice", 112: "/work/bob", 122: "/Users/carol/emdash/worktrees/fix-login", 131: "/work/gemini", 132: "/work/pi"},
		Envs: map[int][]string{102: {"HOME=/home/alice"}, 112: {"HOME=/home/bob"}},
	}
	c := testCollector(t, fp)
	busySince := now.Add(-3 * 24 * time.Hour)
	writePidFile(t, c.Host.UserHome, pidFile{
		PID: 112, SessionID: bobConv, Cwd: "/work/bob", StartedAt: t1.UnixMilli(), Version: "2.1.300",
		Kind: string(agentio.SessionInteractive), Status: "busy", UpdatedAt: busySince.UnixMilli(), StatusUpdatedAt: busySince.UnixMilli(),
	})
	c.Emdash = func() (map[string]emdash.Info, error) {
		return map[string]emdash.Info{"/Users/carol/emdash/worktrees/fix-login": {Task: "fix-login", Status: "in_progress"}}, nil
	}
	ttyActive := now.Add(-5 * 24 * time.Hour)
	c.TTYActivity = func(tty string) time.Time {
		if tty == "ttys001" {
			return ttyActive
		}
		return time.Time{}
	}

	got, err := c.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byPID := map[int]model.Session{}
	for _, s := range got {
		byPID[s.PID] = s
	}
	want := map[int]struct {
		agent  agentio.ID
		source model.Source
	}{
		102: {"claude", model.SourceRunbay},
		112: {"claude", model.SourceCmux},
		122: {"codex", model.SourceEmdash},
		131: {"gemini", model.SourceDirect},
		132: {"pi", model.SourceDirect},
	}
	if len(got) != len(want) {
		t.Fatalf("listed %d sessions %v, want %d", len(got), pids(got), len(want))
	}
	for pid, w := range want {
		s, ok := byPID[pid]
		if !ok || s.Agent != w.agent || s.Source != w.source {
			t.Errorf("pid %d = %+v (listed %v), want %s via %s", pid, s, ok, w.agent, w.source)
		}
	}

	// One cwd lookup per listed session and no more: the batch join reuses
	// Process.Cwd instead of looking it up again.
	if fp.CwdCalls != len(want) {
		t.Errorf("cwd lookups = %d, want %d (one per session)", fp.CwdCalls, len(want))
	}

	alice := byPID[102]
	if alice.RSSBytes != 640<<20 || alice.TTY != "ttys001" || alice.Cwd != "/work/alice" || alice.Kind != agentio.ProcessCLI {
		t.Errorf("alice resources = %+v", alice)
	}
	if alice.ConversationID != aliceConv || alice.Join != agentio.JoinArgv {
		t.Errorf("alice join = %q %s, want %s argv", alice.ConversationID, alice.Join, aliceConv)
	}
	if alice.Idle != now.Sub(ttyActive) || alice.IdleProxy {
		t.Errorf("alice idle = %s proxy %v, want the tty's %s", alice.Idle, alice.IdleProxy, now.Sub(ttyActive))
	}

	bob := byPID[112]
	if bob.ConversationID != bobConv || bob.Join != agentio.JoinExact || bob.Status != agentio.SessionBusy || bob.Version != "2.1.300" {
		t.Errorf("bob join = %+v, want exact busy %s", bob, bobConv)
	}
	if bob.Idle != 0 {
		t.Errorf("bob is busy: idle = %s, want 0 so no threshold reap picks it", bob.Idle)
	}
	if bob.Home == "" {
		t.Error("bob home empty, want the home holding the pid file")
	}

	carol := byPID[122]
	if carol.Task != "fix-login" || carol.TaskStatus != "in_progress" {
		t.Errorf("carol emdash task = %q %q", carol.Task, carol.TaskStatus)
	}
	if !carol.IdleProxy {
		t.Errorf("carol has no tty and no agent activity: idle should be the process-age proxy")
	}

	// Ranked: most idle first.
	for i := 1; i < len(got); i++ {
		if got[i].Idle > got[i-1].Idle {
			t.Fatalf("not ranked by idle: %v", pids(got))
		}
	}
}

func TestSessionsErrors(t *testing.T) {
	t.Parallel()
	c := testCollector(t, &agenttest.FakeProcesses{Err: errors.New("ps failed")})
	if _, err := c.Sessions(context.Background()); err == nil {
		t.Fatal("want the table read error")
	}
	c.Host.Processes = nil
	if _, err := c.Sessions(context.Background()); err == nil {
		t.Fatal("want an error without a process table")
	}

	// An unreadable emdash DB only loses task names.
	ok := testCollector(t, &agenttest.FakeProcesses{List: []agentio.Process{row(5, 1, t0, "claude")}})
	ok.Emdash = func() (map[string]emdash.Info, error) { return nil, errors.New("locked") }
	got, err := ok.Sessions(context.Background())
	if err != nil || len(got) != 1 || got[0].Task != "" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Agents) == 0 || c.Host.Processes == nil || c.Emdash == nil || c.TTYActivity == nil {
		t.Fatalf("collector = %+v", c)
	}
	if !c.now().After(t0) {
		t.Fatal("real clock")
	}
}

func TestRank(t *testing.T) {
	t.Parallel()
	s := []model.Session{
		{PID: 1, Idle: time.Hour, RSSBytes: 100},
		{PID: 2, Idle: 5 * time.Hour, RSSBytes: 50},
		{PID: 3, Idle: 5 * time.Hour, RSSBytes: 900}, // ties idle with #2, more mem
		{PID: 4, Idle: 0, RSSBytes: 999},
	}
	Rank(s)
	// Expected: most-idle first; ties broken by larger memory.
	wantOrder := []int{3, 2, 1, 4}
	for i, pid := range wantOrder {
		if s[i].PID != pid {
			t.Fatalf("position %d = pid %d, want %d (order %v)", i, s[i].PID, pid, pids(s))
		}
	}
}

// pids lists the pids of s in order.
func pids(s []model.Session) []int {
	out := make([]int, len(s))
	for i, x := range s {
		out[i] = x.PID
	}
	return out
}
