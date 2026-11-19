package model

import (
	"testing"
	"time"

	"github.com/ubgo/agentio"
)

// fixed reference time so every case is deterministic.
var now = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

func TestComputeIdle(t *testing.T) {
	tty := now.Add(-2 * time.Hour)
	db := now.Add(-2 * 24 * time.Hour)
	start := now.Add(-6 * 24 * time.Hour)

	cases := []struct {
		name      string
		s         Session
		wantIdle  time.Duration
		wantProxy bool
	}{
		{"tty wins", Session{TTYActive: tty, LastActive: db, StartedAt: start}, 2 * time.Hour, false},
		{"newer agent activity beats an older tty", Session{TTYActive: tty, AgentActive: now.Add(-10 * time.Minute), StartedAt: start}, 10 * time.Minute, false},
		{"newer tty beats older agent activity", Session{TTYActive: tty, AgentActive: now.Add(-5 * time.Hour), StartedAt: start}, 2 * time.Hour, false},
		{"agent activity without a tty", Session{AgentActive: now.Add(-3 * time.Hour), LastActive: db, StartedAt: start}, 3 * time.Hour, false},
		{"busy agent is never idle", Session{Status: agentio.SessionBusy, TTYActive: now.Add(-9 * 24 * time.Hour), StartedAt: start}, 0, false},
		{"waiting agent keeps its idle", Session{Status: agentio.SessionWaiting, TTYActive: tty, StartedAt: start}, 2 * time.Hour, false},
		{"activity in the future clamps to zero", Session{AgentActive: now.Add(time.Minute)}, 0, false},
		{"db fallback when no tty", Session{LastActive: db, StartedAt: start}, 2 * 24 * time.Hour, false},
		{"process-age proxy when neither", Session{StartedAt: start}, 6 * 24 * time.Hour, true},
		{"zero everything", Session{}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.s
			s.ComputeIdle(now)
			if s.Idle != tc.wantIdle {
				t.Errorf("idle = %v, want %v", s.Idle, tc.wantIdle)
			}
			if s.IdleProxy != tc.wantProxy {
				t.Errorf("proxy = %v, want %v", s.IdleProxy, tc.wantProxy)
			}
		})
	}
}

func TestIdleLabel(t *testing.T) {
	cases := []struct {
		idle  time.Duration
		proxy bool
		want  string
	}{
		{0, false, "-"},
		{30 * time.Minute, false, "30m"},
		{5 * time.Hour, false, "5h"},
		{3 * 24 * time.Hour, false, "3d"},
		{5 * 24 * time.Hour, true, "5d*"}, // proxy marker
		{0, true, "-"},                    // no marker on the dash
	}
	if got := (&Session{Status: agentio.SessionBusy}).IdleLabel(); got != "busy" {
		t.Errorf("busy label = %q", got)
	}
	for _, tc := range cases {
		s := Session{Idle: tc.idle, IdleProxy: tc.proxy}
		if got := s.IdleLabel(); got != tc.want {
			t.Errorf("IdleLabel(%v,proxy=%v) = %q, want %q", tc.idle, tc.proxy, got, tc.want)
		}
	}
}

func TestIdleDays(t *testing.T) {
	cases := []struct {
		idle time.Duration
		want int
	}{
		{0, 0},
		{23 * time.Hour, 0},
		{25 * time.Hour, 1},
		{3 * 24 * time.Hour, 3},
		{-time.Hour, 0}, // negative clamps to 0
	}
	for _, tc := range cases {
		s := Session{Idle: tc.idle}
		if got := s.IdleDays(); got != tc.want {
			t.Errorf("IdleDays(%v) = %d, want %d", tc.idle, got, tc.want)
		}
	}
}

func TestLastSeen(t *testing.T) {
	tty := now.Add(-time.Hour)
	db := now.Add(-time.Minute)

	withBoth := Session{TTYActive: tty, LastActive: db}
	if got := withBoth.LastSeen(); !got.Equal(tty) {
		t.Errorf("LastSeen should prefer tty, got %v", got)
	}
	dbOnly := Session{LastActive: db}
	if got := dbOnly.LastSeen(); !got.Equal(db) {
		t.Errorf("LastSeen should fall back to db, got %v", got)
	}
	agentNewer := Session{TTYActive: tty, AgentActive: db}
	if got := agentNewer.LastSeen(); !got.Equal(db) {
		t.Errorf("LastSeen should take the newer agent activity, got %v", got)
	}
	empty := Session{}
	if got := empty.LastSeen(); !got.IsZero() {
		t.Errorf("LastSeen with no signal should be zero, got %v", got)
	}
}
