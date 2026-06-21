// Package model holds the core data types shared across the discovery
// pipeline (discover -> classify -> enrich -> rank -> render -> act).
package model

import "time"

// AgentKind identifies which AI coding agent a process belongs to. v1 ships
// only Claude; the registry (internal/agent) is structured so adding Codex,
// Cursor, etc. is one new detector with no core changes.
type AgentKind string

const (
	AgentClaude  AgentKind = "claude"
	AgentCodex   AgentKind = "codex"
	AgentUnknown AgentKind = "unknown"
)

// Source is which controller launched the session. Classification reflects
// who LAUNCHED the process (parent-chain wins), even when the cwd also maps
// to an emdash workspace.
type Source string

const (
	SourceCmux   Source = "cmux"
	SourceEmdash Source = "emdash"
	SourceDirect Source = "direct"
)

// Session is one running agent process, enriched with everything needed to
// decide whether to reap it.
type Session struct {
	PID       int       `json:"pid"`
	PPID      int       `json:"ppid"`
	Agent     AgentKind `json:"agent"`
	RSSBytes  uint64    `json:"rss_bytes"`
	Cwd       string    `json:"cwd"`
	Source    Source    `json:"source"`
	Command   string    `json:"command"`
	TTY       string    `json:"tty,omitempty"`
	StartedAt time.Time `json:"started_at"`

	// TTYActive is the last I/O time of the controlling terminal (real
	// activity for cmux/direct sessions, which have no DB timestamp).
	TTYActive time.Time `json:"tty_active,omitempty"`

	// emdash enrichment (zero values when not an emdash worktree)
	Task       string    `json:"task,omitempty"`
	TaskStatus string    `json:"task_status,omitempty"`
	LastActive time.Time `json:"last_active,omitempty"`

	// derived
	Idle      time.Duration `json:"idle"`
	IdleProxy bool          `json:"idle_proxy"` // true => Idle is process-age, not true inactivity
}
