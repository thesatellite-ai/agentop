// Package source classifies an agent process by which controller launched it:
// cmux, emdash, runbay, or a plain (direct) terminal.
package source

import (
	"strings"

	"github.com/ubgo/agentio"

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
	markerRunbay     = "runbay"             // prefix of runbay's binaries: runbay-ptyd-<ver>, runbay-daemon, runbay
)

// Classify decides the Source for self, given the process table it was read
// from (agentio Host.Processes) and the process's own cwd.
//
// Signals (first match wins):
//   - own command contains CMUX_CLAUDE_HOOK            -> cmux
//   - an ancestor command contains cmux-agent-resume/cmux -> cmux
//   - an ancestor command contains emdash/Emdash       -> emdash
//   - an ancestor's binary is runbay's (runbay-ptyd…)   -> runbay
//   - cwd under .../emdash/worktrees/                  -> emdash (fallback)
//   - otherwise                                        -> direct
//
// Classification reflects who LAUNCHED the process. A cmux session whose cwd is
// also an emdash workspace is still cmux — the emdash task name is attached as
// enrichment elsewhere, not as the source. agentio walks the parent chain
// (Ancestors); which launcher a chain belongs to is agentop's knowledge.
func Classify(self agentio.Process, table []agentio.Process, cwd string) model.Source {
	if strings.Contains(self.CommandString(), markerCmuxHook) {
		return model.SourceCmux
	}
	for _, parent := range agentio.Ancestors(table, self.PID, maxHops) {
		cmd := parent.CommandString()
		switch {
		case strings.Contains(cmd, markerCmuxResume), containsWord(cmd, markerCmuxName):
			return model.SourceCmux
		case strings.Contains(cmd, markerEmdashLow), strings.Contains(cmd, markerEmdashCap):
			return model.SourceEmdash
		case isRunbay(parent):
			return model.SourceRunbay
		}
	}
	if strings.Contains(cwd, markerEmdashWork) {
		return model.SourceEmdash
	}
	return model.SourceDirect
}

// isRunbay matches an ancestor that is one of runbay's own binaries. Runbay
// session shells run under its pty daemon (`/…/.runbay/bin/runbay-ptyd-0.1.0
// ptyd --daemon` → `zsh -l` → agent). Only the executable's basename is
// checked, so a command that merely mentions a runbay path (an ssh into a
// runbay checkout) does not match.
func isRunbay(p agentio.Process) bool {
	return strings.HasPrefix(agentio.ProcessBase(argv0(p)), markerRunbay)
}

// argv0 is the executable a row names: Argv[0], else Exe.
func argv0(p agentio.Process) string {
	if len(p.Argv) > 0 {
		return p.Argv[0]
	}
	return p.Exe
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
