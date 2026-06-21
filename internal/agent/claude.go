package agent

import (
	"strings"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// claudeDetector matches the Claude Code CLI. It identifies the CLI by its
// binary path (".../bin/claude") and excludes the desktop app, the
// claude-in-chrome MCP bridge, and Application Support helpers — all of which
// also contain "claude" in their command line but are not reapable sessions.
type claudeDetector struct{}

func (claudeDetector) Kind() model.AgentKind { return model.AgentClaude }

// Detection signatures for the Claude Code CLI.
const (
	claudeBinName       = "claude"       // expected argv0 basename of the CLI launcher
	claudeBinPathInfix  = "/bin/claude " // launcher path followed by args (node-wrapped invocations)
	claudeBinPathSuffix = "/bin/claude"  // launcher path with no trailing args
)

// claudeExcludes are substrings that mark a "claude" process we must NOT treat
// as a reapable CLI session — the desktop app, the in-chrome MCP bridge, and
// app-support helpers all contain "claude" but are not agent sessions.
var claudeExcludes = []string{
	"Claude.app",
	"claude-in-chrome",
	"Application Support/Claude",
}

func (claudeDetector) Matches(command string) bool {
	if command == "" {
		return false
	}
	for _, ex := range claudeExcludes {
		if strings.Contains(command, ex) {
			return false
		}
	}
	// argv0 is the first whitespace-delimited token; the CLI launcher is the
	// "claude" binary on PATH (e.g. ~/.local/bin/claude).
	argv0 := command
	if i := strings.IndexByte(command, ' '); i >= 0 {
		argv0 = command[:i]
	}
	base := argv0
	if i := strings.LastIndexByte(argv0, '/'); i >= 0 {
		base = argv0[i+1:]
	}
	if base == claudeBinName {
		return true
	}
	// Fallback: explicit bin/claude path anywhere in the command (covers
	// node-wrapped invocations that keep the launcher path in args).
	return strings.Contains(command, claudeBinPathInfix) || strings.HasSuffix(command, claudeBinPathSuffix)
}
