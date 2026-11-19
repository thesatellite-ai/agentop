package collect

import (
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestMatcherParity runs the matcher tables of agentop's former
// internal/agent package (TestClassify, TestNonSession, TestIsSession, as on
// branch safety-fixes) through agentio's classifiers, the code agentop now
// lists and reaps with. want is the agent that claims the row ("" = none) and
// session whether agentop lists it as a reapable conversation. Every row
// keeps the expectation it had before the switch.
func TestMatcherParity(t *testing.T) {
	t.Parallel()
	agents := testCollector(t, &agenttest.FakeProcesses{}).Agents
	for _, tc := range []struct {
		name    string
		command string
		want    agentio.ID
		session bool
	}{
		// former TestClassify.
		{"claude launcher", "/Users/x/.local/bin/claude --resume abc", "claude", true},
		{"claude bin path infix", "/usr/bin/node /opt/bin/claude --foo", "claude", true},
		{"claude desktop app excluded", "/Applications/Claude.app/Contents/MacOS/Claude", "", false},
		{"claude-in-chrome excluded", "/x/claude-in-chrome/server", "", false},
		{"codex launcher", "/opt/homebrew/bin/codex", "codex", true},
		{"codex via node pkg path", "node /opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js", "codex", false},
		{"unrelated process", "/usr/bin/vim main.go", "", false},
		{"empty", "", "", false},

		// former TestNonSession (a reason there = not a session here).
		{"claude session", "claude --resume abc --dangerously-skip-permissions", "claude", true},
		{"claude session with flags only", "/Users/x/.local/bin/claude --chrome --remote-control", "claude", true},
		{"claude mcp serve", "claude mcp serve", "claude", false},
		{"claude mcp serve full path", "/Users/x/.local/bin/claude mcp serve", "claude", false},
		{"claude mcp after a flag", "claude --debug mcp serve", "claude", false},
		{"claude node-wrapped mcp serve", "/usr/bin/node /opt/bin/claude mcp serve", "claude", false},
		{"claude resume id is not mcp", "claude --resume mcp-notes", "claude", true},
		{"codex session", "/opt/homebrew/bin/codex", "codex", true},
		{"codex resume", "codex resume 0199-abc", "codex", true},
		{"codex exec", "codex exec 'fix the tests'", "codex", true},
		{"codex app-server", "/opt/homebrew/bin/codex app-server", "codex", false},
		{"codex app-server after flag value", "codex -c model=o3 app-server --listen unix://x", "codex", false},
		{"codex mcp-server", "codex mcp-server", "codex", false},
		{"codex npm launcher", "node /opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js", "codex", false},
		{"codex npm launcher via bun", "/usr/local/bin/bun /x/@openai/codex/bin/codex.js resume", "codex", false},
		{"codex native child of launcher", "/opt/homebrew/lib/node_modules/@openai/codex/node_modules/@openai/codex-darwin-arm64/vendor/aarch64-apple-darwin/codex/codex", "codex", true},
		{"not an agent", "/usr/bin/python3 headroom mcp serve", "", false},

		// former TestIsSession.
		{"is-session claude", "claude --resume abc", "claude", true},
		{"is-session codex app-server", "codex app-server", "codex", false},
		{"is-session codex launcher", "node /x/@openai/codex/bin/codex.js", "codex", false},

		// Rows the safety-fixes branch handled by not matching them.
		{"claude shell snapshot", "/bin/zsh -c source /Users/alice/.claude/shell-snapshots/snapshot-zsh-1.sh 2>/dev/null || true", "", false},
		{"claude desktop bundled CLI", "/Users/alice/Library/Application Support/Claude/claude-code/2.1.300/claude --output-format stream-json", "", false},
	} {
		info, ok := agentio.ClassifyProcess(agents, row(100, 1, t0, tc.command))
		got := agentio.ID("")
		if ok {
			got = info.Agent
		}
		if got != tc.want || info.Session != tc.session {
			t.Errorf("%s: %q = %q session %v, want %q session %v", tc.name, tc.command, got, info.Session, tc.want, tc.session)
		}
	}
}
