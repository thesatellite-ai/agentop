package agent

import (
	"testing"

	"github.com/thesatellite-ai/agentop/internal/model"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name    string
		command string
		want    model.AgentKind
	}{
		{"claude launcher", "/Users/x/.local/bin/claude --resume abc", model.AgentClaude},
		{"claude bin path infix", "/usr/bin/node /opt/bin/claude --foo", model.AgentClaude},
		{"claude desktop app excluded", "/Applications/Claude.app/Contents/MacOS/Claude", model.AgentUnknown},
		{"claude-in-chrome excluded", "/x/claude-in-chrome/server", model.AgentUnknown},
		{"codex launcher", "/opt/homebrew/bin/codex", model.AgentCodex},
		{"codex via node pkg path", "node /opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js", model.AgentCodex},
		{"unrelated process", "/usr/bin/vim main.go", model.AgentUnknown},
		{"empty", "", model.AgentUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.command); got != tc.want {
				t.Fatalf("Classify(%q) = %q, want %q", tc.command, got, tc.want)
			}
		})
	}
}
