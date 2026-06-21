package source

import (
	"strconv"
	"testing"

	"github.com/thesatellite-ai/agentop/internal/discover"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// index builds a pid->Proc map from (pid, ppid, command) triples.
func index(rows ...[3]string) map[int]discover.Proc {
	m := map[int]discover.Proc{}
	for _, r := range rows {
		pid, _ := strconv.Atoi(r[0])
		ppid, _ := strconv.Atoi(r[1])
		m[pid] = discover.Proc{PID: pid, PPID: ppid, Command: r[2]}
	}
	return m
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		idx  map[int]discover.Proc
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.pid, tc.idx, tc.cwd); got != tc.want {
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
