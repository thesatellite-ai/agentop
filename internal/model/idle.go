package model

import (
	"fmt"
	"time"
)

// DefaultReapIdleDays is the default idle threshold (in days) for the `reap`
// command and the TUI reap-by-idle prompt. Defined here as the single source of
// truth so the CLI flag default and the TUI input can't drift apart.
const DefaultReapIdleDays = 2

// ComputeIdle sets s.Idle and s.IdleProxy. The controlling-tty I/O time is the
// single source of truth for activity across every source (cmux, direct, AND
// emdash). emdash's DB last_interacted_at is intentionally NOT used for idle —
// it lags real activity by hours-to-days (it's only written on certain logged
// events), so trusting it would mark an actively-working agent as idle.
//
// Priority:
//
//  1. tty I/O time (TTYActive) — ground truth of "anything flowing through this
//     session's terminal";
//  2. emdash DB time (LastActive) — fallback ONLY when a session has no
//     controlling tty (rare);
//  3. process start time (StartedAt) — a PROXY (IdleProxy=true), last resort;
//     rendered with a trailing "*" and never presented as true inactivity.
func (s *Session) ComputeIdle(now time.Time) {
	best := s.TTYActive
	if best.IsZero() {
		best = s.LastActive // fallback: no tty
	}
	if !best.IsZero() {
		s.Idle = now.Sub(best)
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

// IdleLabel renders the idle duration compactly. Proxy values get a trailing
// "*" so age-based idle is visually distinct from true inactivity.
func (s *Session) IdleLabel() string {
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

// LastSeen returns the activity timestamp behind Idle: tty I/O, or the emdash
// DB time when there's no tty. Zero when only the process-age proxy applies.
func (s *Session) LastSeen() time.Time {
	if !s.TTYActive.IsZero() {
		return s.TTYActive
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
