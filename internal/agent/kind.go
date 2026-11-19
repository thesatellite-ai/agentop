package agent

import (
	"path"
	"slices"
	"strings"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// Reasons NonSession gives for an agent process that is not a conversation.
// They are shown to the user, so they say what the process is.
const (
	ReasonClaudeMCP   = "claude mcp server"
	ReasonCodexServer = "codex server"
	ReasonCodexLaunch = "codex launcher"
)

// Argv words that mark a non-session agent process. These are the agents'
// own subcommand names, kept verbatim.
const (
	claudeSubMCP   = "mcp"   // `claude mcp serve|add|list…`: an MCP server or a config command
	claudeSubServe = "serve" // second word of `claude mcp serve`
	codexScript    = "codex.js"
)

// codexServerSubs are Codex subcommands that run a server, not a conversation:
// the app-server daemon serves many threads (the TUI reattaches to it), and
// mcp-server / mcp serve tools to other clients. Killing one is never a
// "reap an idle session" action.
var codexServerSubs = []string{"app-server", "mcp-server", "mcp"}

// interpreters run the npm Codex launcher script (`node …/bin/codex.js`). The
// launcher only spawns the native codex binary, which is the real session; its
// pid is a wrapper whose death takes the child's stdio with it.
var interpreters = []string{"node", "bun", "deno"}

// NonSession reports why an agent process is not a reapable conversation, or
// "" when it is one (or is not an agent at all). Listing and reaping both skip
// processes with a reason, so a server or launcher is never signalled.
func NonSession(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	switch Classify(command) {
	case model.AgentClaude:
		if claudeIsMCP(fields) {
			return ReasonClaudeMCP
		}
	case model.AgentCodex:
		if slices.Contains(interpreters, path.Base(fields[0])) ||
			(len(fields) > 1 && strings.HasSuffix(fields[1], codexScript)) {
			return ReasonCodexLaunch
		}
		// Any argv word is checked, not just the first positional: a flag
		// value in between (`codex -c k=v app-server`) must not hide the
		// subcommand. A false match only hides a process, it never kills one.
		for _, f := range fields[1:] {
			if slices.Contains(codexServerSubs, f) {
				return ReasonCodexServer
			}
		}
	}
	return ""
}

// IsSession reports whether command is an agent conversation process: a
// matched agent that NonSession does not exclude.
func IsSession(command string) bool {
	return IsAgent(command) && NonSession(command) == ""
}

// claudeIsMCP reports `claude mcp …`: the first word after the claude binary
// that is not a flag is "mcp", or the words "mcp serve" appear together.
func claudeIsMCP(fields []string) bool {
	bin := slices.IndexFunc(fields, func(f string) bool { return path.Base(f) == claudeBinName })
	if bin < 0 {
		bin = 0
	}
	for _, f := range fields[bin+1:] {
		if !strings.HasPrefix(f, "-") {
			if f == claudeSubMCP {
				return true
			}
			break
		}
	}
	for i := bin + 1; i+1 < len(fields); i++ {
		if fields[i] == claudeSubMCP && fields[i+1] == claudeSubServe {
			return true
		}
	}
	return false
}
