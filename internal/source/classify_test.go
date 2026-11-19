package source

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ubgo/agentio"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// index builds a process table from (pid, ppid, command) triples, shaped the
// way agentio's darwin reader fills a row: Argv split on spaces, Exe =
// argv[0], CommandLine the raw command column.
func index(rows ...[3]string) []agentio.Process {
	var table []agentio.Process
	for _, r := range rows {
		pid, _ := strconv.Atoi(r[0])
		ppid, _ := strconv.Atoi(r[1])
		argv := strings.Fields(r[2])
		p := agentio.Process{PID: pid, PPID: ppid, Argv: argv, CommandLine: r[2]}
		if len(argv) > 0 {
			p.Exe = argv[0]
		}
		table = append(table, p)
	}
	return table
}

// row returns pid's row in table (zero when absent).
func row(table []agentio.Process, pid int) agentio.Process {
	if i := slices.IndexFunc(table, func(p agentio.Process) bool { return p.PID == pid }); i >= 0 {
		return table[i]
	}
	return agentio.Process{PID: pid}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		idx  []agentio.Process
		pid  int
		cwd  string
		want model.Source
	}{
		{
			name: "cmux via own hook",
			idx:  index([3]string{"10", "1", "claude --settings {CMUX_CLAUDE_HOOK_CMUX_BIN}"}),
			pid:  10, want: model.SourceCmux,
		},
		{
			name: "cmux via parent resume shell",
			idx: index(
				[3]string{"20", "21", "claude --resume x"},
				[3]string{"21", "1", "/bin/zsh /T/cmux-agent-resume/claude-abc.zsh"},
			),
			pid: 20, want: model.SourceCmux,
		},
		{
			name: "emdash via ancestor",
			idx: index(
				[3]string{"30", "31", "claude"},
				[3]string{"31", "32", "/bin/zsh"},
				[3]string{"32", "1", "/Applications/Emdash.app/Contents/MacOS/Emdash"},
			),
			pid: 30, want: model.SourceEmdash,
		},
		{
			name: "emdash via worktree cwd fallback",
			idx:  index([3]string{"40", "1", "claude"}),
			pid:  40, cwd: "/Users/x/emdash/worktrees/proj-abc", want: model.SourceEmdash,
		},
		{
			name: "direct: no controller, plain cwd",
			idx:  index([3]string{"50", "1", "claude"}),
			pid:  50, cwd: "/Users/x/code/proj", want: model.SourceDirect,
		},
		// Runbay fixtures: the chain seen on a live Mac, paths scrubbed.
		// claude -> zsh -l -> runbay-ptyd-<ver> ptyd --daemon -> launchd.
		{
			name: "runbay via pty daemon (macOS)",
			idx: index(
				[3]string{"60", "61", "claude --resume 0f79d084 --dangerously-skip-permissions --chrome"},
				[3]string{"61", "62", "/bin/zsh -l"},
				[3]string{"62", "1", "/Users/x/.runbay/bin/runbay-ptyd-0.1.0 ptyd --daemon"},
			),
			pid: 60, cwd: "/Users/x/code/proj", want: model.SourceRunbay,
		},
		{
			name: "runbay via pty daemon run as a subcommand (linux)",
			idx: index(
				[3]string{"70", "71", "claude"},
				[3]string{"71", "72", "-zsh"},
				[3]string{"72", "1", "/usr/local/bin/runbay ptyd --daemon"},
			),
			pid: 70, want: model.SourceRunbay,
		},
		{
			name: "runbay node-wrapped agent one hop deeper",
			idx: index(
				[3]string{"80", "81", "node /opt/bin/claude"},
				[3]string{"81", "82", "/bin/bash -c claude"},
				[3]string{"82", "83", "/bin/zsh -l"},
				[3]string{"83", "1", "/Users/x/.runbay/bin/runbay-ptyd-0.2.0 ptyd --daemon"},
			),
			pid: 80, want: model.SourceRunbay,
		},
		{
			name: "plain Terminal login shell stays direct",
			idx: index(
				[3]string{"90", "91", "claude --resume ca8845bd"},
				[3]string{"91", "92", "-zsh"},
				[3]string{"92", "93", "login -pfl x /bin/bash -c exec -la zsh /bin/zsh"},
				[3]string{"93", "1", "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal"},
			),
			pid: 90, want: model.SourceDirect,
		},
		{
			name: "a runbay path in an ancestor's args is not runbay",
			idx: index(
				[3]string{"95", "96", "claude"},
				[3]string{"96", "1", "ssh host cd /space/projects/runbay/wt-1 && claude"},
			),
			pid: 95, want: model.SourceDirect,
		},
		{
			name: "runbay app daemon as the launcher",
			idx: index(
				[3]string{"97", "98", "claude"},
				[3]string{"98", "1", "/Applications/Runbay.app/Contents/MacOS/runbay-daemon serve"},
			),
			pid: 97, want: model.SourceRunbay,
		},
		{
			name: "cmux wins over runbay when nearer",
			idx: index(
				[3]string{"100", "101", "claude"},
				[3]string{"101", "102", "/bin/zsh /T/cmux-agent-resume/claude-abc.zsh"},
				[3]string{"102", "1", "/Users/x/.runbay/bin/runbay-ptyd-0.1.0 ptyd --daemon"},
			),
			pid: 100, want: model.SourceCmux,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(row(tc.idx, tc.pid), tc.idx, tc.cwd); got != tc.want {
				t.Errorf("Classify = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestContainsWord(t *testing.T) {
	cases := []struct {
		s, w string
		want bool
	}{
		{"a cmux b", "cmux", true},
		{"/bin/cmux", "cmux", true},
		{"cmuxterm", "cmux", false}, // boundary: not a whole word
		{"precmux", "cmux", false},
		{"cmux", "cmux", true},
		{"nothing", "cmux", false},
	}
	for _, tc := range cases {
		if got := containsWord(tc.s, tc.w); got != tc.want {
			t.Errorf("containsWord(%q,%q) = %v, want %v", tc.s, tc.w, got, tc.want)
		}
	}
}
