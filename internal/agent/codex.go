package agent

import (
	"strings"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// codexDetector matches the OpenAI Codex CLI. Codex ships as an npm package
// (`@openai/codex`) whose `bin/codex.js` is run via node, so a running process
// shows up either as the `codex` launcher (argv0 basename) or as node executing
// the package's entry script — we match both.
type codexDetector struct{}

func (codexDetector) Kind() model.AgentKind { return model.AgentCodex }

// Detection signatures for the Codex CLI.
const (
	codexBinName = "codex"         // argv0 basename of the launcher / global bin shim
	codexPkgPath = "@openai/codex" // node_modules path present when run via node (most reliable)
)

func (codexDetector) Matches(command string) bool {
	if command == "" {
		return false
	}
	// Most reliable: the npm package path appears in the command line whenever
	// node executes the entry script directly.
	if strings.Contains(command, codexPkgPath) {
		return true
	}
	// Otherwise match the launcher by argv0 basename.
	argv0 := command
	if i := strings.IndexByte(command, ' '); i >= 0 {
		argv0 = command[:i]
	}
	base := argv0
	if i := strings.LastIndexByte(argv0, '/'); i >= 0 {
		base = argv0[i+1:]
	}
	return base == codexBinName
}
