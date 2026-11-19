//go:build darwin || linux

// Package discover reads the host facts agentop needs beyond the process
// table: terminal activity and physical memory. The process table itself
// (pid, ppid, start time, RSS, tty, command line, cwd) comes from agentio.
package discover

import (
	"path/filepath"
	"time"
)

// devDir is where terminal device files live; agentio reports a tty
// without it ("ttys004", "pts/3").
const devDir = "/dev"

// TTYActivity returns the last I/O time of a process's controlling terminal
// (max of the device's atime and mtime) — a real "is this session doing
// anything" signal that works for any terminal-attached session (cmux, direct,
// emdash panes). Returns the zero time if there's no controlling tty.
//
// This is how w(1)/finger(1) compute idle: the pty device mtime advances on
// output, atime on input.
func TTYActivity(tty string) time.Time {
	if tty == "" {
		return time.Time{}
	}
	return ttyActivity(filepath.Join(devDir, tty))
}
