package tui

import (
	"strings"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// fuzzyMatch reports whether every rune of query appears in target in order
// (a subsequence match — the same model editors use for quick-open). Both
// arguments must already be lower-cased by the caller; an empty query matches
// everything.
func fuzzyMatch(query, target string) bool {
	if query == "" {
		return true
	}
	qi := 0
	q := []rune(query)
	for _, tr := range target {
		if tr == q[qi] {
			qi++
			if qi == len(q) {
				return true
			}
		}
	}
	return false
}

// sessionHaystack builds the lower-cased string the quick-filter matches
// against: the agent, source, task, and cwd columns (the user-meaningful text
// fields). PID/memory/idle are excluded — they're not text the user types.
func sessionHaystack(s model.Session) string {
	return strings.ToLower(string(s.Agent) + " " + string(s.Source) + " " + s.Task + " " + s.Cwd)
}
