// Package source classifies an agent process by which controller launched it:
// cmux, emdash, or a plain (direct) terminal.
package source

import (
	"strings"

	"github.com/thesatellite-ai/agentop/internal/discover"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// maxHops bounds how far up the parent chain we walk looking for a controller.
const maxHops = 4

// Detection signatures — substrings agentop matches against process command
// lines (and the cwd) to identify a controller. These are upstream/wire-format
// markers emitted by cmux and emdash, kept verbatim; they are named here so the
// classification logic reads declaratively and the markers live in one place.
const (
	markerCmuxHook   = "CMUX_CLAUDE_HOOK"   // env hook cmux bakes into the Claude --settings
	markerCmuxResume = "cmux-agent-resume"  // cmux's session-resume shell, seen in the parent chain
	markerCmuxName   = "cmux"               // generic cmux launcher token (word-boundary matched)
	markerEmdashLow  = "emdash"             // emdash launcher/path (lowercase)
	markerEmdashCap  = "Emdash"             // emdash app name (capitalized) in the parent chain
	markerEmdashWork = "/emdash/worktrees/" // emdash worktree path prefix (cwd fallback signal)
)

// Classify decides the Source for the process `pid`, given a pid->Proc index of
// every process and the process's own cwd.
//
// Signals (first match wins):
//   - own command contains CMUX_CLAUDE_HOOK            -> cmux
//   - an ancestor command contains cmux-agent-resume/cmux -> cmux
//   - an ancestor command contains emdash/Emdash       -> emdash
//   - cwd under .../emdash/worktrees/                  -> emdash (fallback)
//   - otherwise                                        -> direct
//
// Classification reflects who LAUNCHED the process. A cmux session whose cwd is
// also an emdash workspace is still cmux — the emdash task name is attached as
// enrichment elsewhere, not as the source.
func Classify(pid int, index map[int]discover.Proc, cwd string) model.Source {
	self, ok := index[pid]
	if ok && strings.Contains(self.Command, markerCmuxHook) {
		return model.SourceCmux
	}
	cur := pid
	for hop := 0; hop < maxHops; hop++ {
		p, ok := index[cur]
		if !ok || p.PPID == 0 || p.PPID == cur {
			break
		}
		parent, ok := index[p.PPID]
		if !ok {
			break
		}
		cmd := parent.Command
		switch {
		case strings.Contains(cmd, markerCmuxResume), containsWord(cmd, markerCmuxName):
			return model.SourceCmux
		case strings.Contains(cmd, markerEmdashLow), strings.Contains(cmd, markerEmdashCap):
			return model.SourceEmdash
		}
		cur = p.PPID
	}
	if strings.Contains(cwd, markerEmdashWork) {
		return model.SourceEmdash
	}
	return model.SourceDirect
}

// containsWord avoids matching "cmux" inside an unrelated longer token by
// requiring a non-alphanumeric boundary on both sides.
func containsWord(s, word string) bool {
	idx := 0
	for {
		i := strings.Index(s[idx:], word)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(word)
		leftOK := start == 0 || !isAlnum(s[start-1])
		rightOK := end == len(s) || !isAlnum(s[end])
		if leftOK && rightOK {
			return true
		}
		idx = end
	}
}

func isAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
