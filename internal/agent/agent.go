// Package agent holds the pluggable detector registry that maps a running
// process to an AgentKind. v1 registers only Claude, but all discovery flows
// through the registry so adding Codex/Cursor/etc. later is one new Detector.
package agent

import "github.com/thesatellite-ai/agentop/internal/model"

// Detector decides whether a process (by its full command line) is an
// instance of a particular agent.
type Detector interface {
	Kind() model.AgentKind
	// Matches reports whether the given command line is this agent.
	Matches(command string) bool
}

// registry is the ordered list of detectors consulted during discovery. Add a
// new agent by appending its detector here — discovery, source classification,
// idle, and the TUI all flow from the registry with no further changes.
var registry = []Detector{
	claudeDetector{},
	codexDetector{},
}

// Classify returns the AgentKind for a command line, or AgentUnknown.
func Classify(command string) model.AgentKind {
	for _, d := range registry {
		if d.Matches(command) {
			return d.Kind()
		}
	}
	return model.AgentUnknown
}

// IsAgent reports whether the command belongs to any registered agent.
func IsAgent(command string) bool {
	return Classify(command) != model.AgentUnknown
}

// Register adds a detector (for future agents / tests).
func Register(d Detector) { registry = append(registry, d) }
