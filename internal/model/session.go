// Package model holds the core data types shared across the discovery
// pipeline (discover -> classify -> enrich -> rank -> render -> act).
package model

import (
	"time"

	"github.com/ubgo/agentio"
)

// Source is which controller launched the session. Classification reflects
// who LAUNCHED the process (parent-chain wins), even when the cwd also maps
// to an emdash workspace.
type Source string

const (
	SourceCmux   Source = "cmux"
	SourceEmdash Source = "emdash"
	SourceRunbay Source = "runbay"
	SourceDirect Source = "direct"
)

// Sources lists every Source, in the order the UI shows them.
var Sources = []Source{SourceEmdash, SourceCmux, SourceRunbay, SourceDirect}

// Session is one running agent conversation process, enriched with
// everything needed to decide whether to reap it. Which agent it is, and
// what the agent recorded about it, come from agentio.
type Session struct {
	PID   int        `json:"pid"`
	PPID  int        `json:"ppid"`
	Agent agentio.ID `json:"agent"`
	// Kind is the agent process kind agentio classified (always a
	// conversation kind here: cli or exec).
	Kind      agentio.ProcessKind `json:"kind,omitempty"`
	RSSBytes  uint64              `json:"rss_bytes"`
	Cwd       string              `json:"cwd"`
	Source    Source              `json:"source"`
	Command   string              `json:"command"`
	TTY       string              `json:"tty,omitempty"`
	StartedAt time.Time           `json:"started_at"`

	// TTYActive is the last I/O time of the controlling terminal (real
	// activity for cmux/direct sessions, which have no DB timestamp).
	TTYActive time.Time `json:"tty_active,omitempty"`

	// What the agent recorded about this process (agentio
	// ProcessSession); zero values when nothing ties it to a conversation.
	ConversationID string                 `json:"conversation_id,omitempty"`
	Home           string                 `json:"home,omitempty"`
	Version        string                 `json:"version,omitempty"`
	Status         agentio.SessionStatus  `json:"status"`
	AgentActive    time.Time              `json:"agent_active,omitempty"`
	Join           agentio.JoinConfidence `json:"join"`

	// emdash enrichment (zero values when not an emdash worktree)
	Task       string    `json:"task,omitempty"`
	TaskStatus string    `json:"task_status,omitempty"`
	LastActive time.Time `json:"last_active,omitempty"`

	// derived
	Idle      time.Duration `json:"idle"`
	IdleProxy bool          `json:"idle_proxy"` // true => Idle is process-age, not true inactivity
}
