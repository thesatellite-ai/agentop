package tui

import (
	"sort"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// SortField enumerates the columns the user can sort by.
type SortField int

const (
	SortIdle SortField = iota
	SortMem
	SortPID
	SortAgent
	SortSource
	SortTask
	SortCwd
)

var sortFields = []SortField{SortIdle, SortMem, SortPID, SortAgent, SortSource, SortTask, SortCwd}

func (f SortField) String() string {
	switch f {
	case SortMem:
		return "memory"
	case SortPID:
		return "pid"
	case SortAgent:
		return "agent"
	case SortSource:
		return "source"
	case SortTask:
		return "task"
	case SortCwd:
		return "cwd"
	default:
		return "idle"
	}
}

// less returns the ascending comparison for the field.
func (f SortField) less(a, b model.Session) bool {
	switch f {
	case SortMem:
		return a.RSSBytes < b.RSSBytes
	case SortPID:
		return a.PID < b.PID
	case SortAgent:
		return a.Agent < b.Agent
	case SortSource:
		return a.Source < b.Source
	case SortTask:
		return a.Task < b.Task
	case SortCwd:
		return a.Cwd < b.Cwd
	default: // idle
		return a.Idle < b.Idle
	}
}

// sortSessions sorts in place by field + direction.
func sortSessions(s []model.Session, f SortField, asc bool) {
	sort.SliceStable(s, func(i, j int) bool {
		if asc {
			return f.less(s[i], s[j])
		}
		return f.less(s[j], s[i])
	})
}
