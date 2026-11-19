package agent

import "testing"

func TestNonSession(t *testing.T) {
	cases := []struct {
		name    string
		command string
		want    string
	}{
		{"claude session", "claude --resume abc --dangerously-skip-permissions", ""},
		{"claude session with flags only", "/Users/x/.local/bin/claude --chrome --remote-control", ""},
		{"claude mcp serve", "claude mcp serve", ReasonClaudeMCP},
		{"claude mcp serve full path", "/Users/x/.local/bin/claude mcp serve", ReasonClaudeMCP},
		{"claude mcp after a flag", "claude --debug mcp serve", ReasonClaudeMCP},
		{"claude node-wrapped mcp serve", "/usr/bin/node /opt/bin/claude mcp serve", ReasonClaudeMCP},
		{"claude resume id is not mcp", "claude --resume mcp-notes", ""},
		{"codex session", "/opt/homebrew/bin/codex", ""},
		{"codex resume", "codex resume 0199-abc", ""},
		{"codex exec", "codex exec 'fix the tests'", ""},
		{"codex app-server", "/opt/homebrew/bin/codex app-server", ReasonCodexServer},
		{"codex app-server after flag value", "codex -c model=o3 app-server --listen unix://x", ReasonCodexServer},
		{"codex mcp-server", "codex mcp-server", ReasonCodexServer},
		{"codex npm launcher", "node /opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js", ReasonCodexLaunch},
		{"codex npm launcher via bun", "/usr/local/bin/bun /x/@openai/codex/bin/codex.js resume", ReasonCodexLaunch},
		{"codex native child of launcher", "/opt/homebrew/lib/node_modules/@openai/codex/node_modules/@openai/codex-darwin-arm64/vendor/aarch64-apple-darwin/codex/codex", ""},
		{"not an agent", "/usr/bin/python3 headroom mcp serve", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NonSession(tc.command); got != tc.want {
				t.Fatalf("NonSession(%q) = %q, want %q", tc.command, got, tc.want)
			}
		})
	}
}

func TestIsSession(t *testing.T) {
	cases := map[string]bool{
		"claude --resume abc":                true,
		"claude mcp serve":                   false,
		"codex app-server":                   false,
		"node /x/@openai/codex/bin/codex.js": false,
		"/usr/bin/vim main.go":               false,
	}
	for cmd, want := range cases {
		if got := IsSession(cmd); got != want {
			t.Errorf("IsSession(%q) = %v, want %v", cmd, got, want)
		}
	}
}
