package model

import (
	"fmt"
	"time"

	"github.com/ubgo/agentio"
)

// DefaultReapIdleDays is the default idle threshold (in days) for the `reap`
// command and the TUI reap-by-idle prompt. Defined here as the single source of
// truth so the CLI flag default and the TUI input can't drift apart.
const DefaultReapIdleDays = 2

// ComputeIdle sets s.Idle and s.IdleProxy. Activity comes from two clocks
// that each miss a case, so the newer one wins:
//
//   - the controlling-tty I/O time (TTYActive): anything flowing through the
//     session's terminal, the w(1) method;
//   - the agent's own last-activity time (AgentActive, agentio
//     ProcessSession.LastActivity): it moves while a turn streams with a quiet
//     screen and works for sessions without a tty.
//
// A session the agent reports busy (Claude's sessions/<pid>.json status) is
// working on a turn right now, so its idle is zero whatever the clocks say:
// a threshold reap never picks it. Taking the newer clock and zeroing busy
// sessions can only make a session look less idle than the tty alone says,
// never more, so the change cannot widen what a reap selects.
//
// emdash's DB last_interacted_at is a fallback ONLY when neither clock is
// known: it lags real activity by hours-to-days (it's only written on
// certain logged events). The process start time is the last resort, a
// PROXY (IdleProxy=true) rendered with a trailing "*".
func (s *Session) ComputeIdle(now time.Time) {
	if s.Status == agentio.SessionBusy {
		s.Idle = 0
		s.IdleProxy = false
		return
	}
	best := s.LastSeen()
	if !best.IsZero() {
		s.Idle = max(now.Sub(best), 0)
		s.IdleProxy = false
		return
	}
	if !s.StartedAt.IsZero() {
		s.Idle = now.Sub(s.StartedAt)
		s.IdleProxy = true
		return
	}
	s.Idle = 0
	s.IdleProxy = true
}

// idleLabelBusy is the idle label of a session its agent reports busy.
const idleLabelBusy = "busy"

// IdleLabel renders the idle duration compactly; "busy" when the agent
// reports it is working on a turn. Proxy values get a trailing
// "*" so age-based idle is visually distinct from true inactivity.
func (s *Session) IdleLabel() string {
	if s.Status == agentio.SessionBusy {
		return idleLabelBusy
	}
	d := s.Idle
	var out string
	switch {
	case d <= 0:
		out = "-"
	case d < time.Hour:
		out = fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		out = fmt.Sprintf("%dh", int(d.Hours()))
	default:
		out = fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if s.IdleProxy && out != "-" {
		out += "*"
	}
	return out
}

// LastSeen returns the activity timestamp behind Idle: the newer of tty I/O
// and the agent's own activity time, else the emdash DB time. Zero when only
// the process-age proxy applies.
func (s *Session) LastSeen() time.Time {
	best := s.TTYActive
	if s.AgentActive.After(best) {
		best = s.AgentActive
	}
	if !best.IsZero() {
		return best
	}
	return s.LastActive
}

// IdleDays returns whole days idle (floor). Used by reap thresholds.
func (s *Session) IdleDays() int {
	if s.Idle <= 0 {
		return 0
	}
	return int(s.Idle.Hours() / 24)
}
