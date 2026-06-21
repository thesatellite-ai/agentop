//go:build darwin || linux

package discover

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TTYActivity returns the last I/O time of a process's controlling terminal
// (max of the device's atime and mtime) — a real "is this session doing
// anything" signal that works for any terminal-attached session (cmux, direct,
// emdash panes). Returns the zero time if there's no controlling tty.
//
// This is how w(1)/finger(1) compute idle: the pty device mtime advances on
// output, atime on input.
func TTYActivity(tty string) time.Time {
	if tty == "" || tty == "??" || tty == "?" || tty == "-" {
		return time.Time{}
	}
	return ttyActivity("/dev/" + tty)
}

// cwdWorkers bounds concurrent lsof calls. lsof per-PID is the latency hotspot
// (there can be 25+ agent processes), so resolve them in parallel.
const cwdWorkers = 12

// Cwd resolves a single process's working directory via lsof.
func Cwd(pid int) string {
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimSpace(line[1:])
		}
	}
	return ""
}

// Cwds resolves working directories for many PIDs concurrently.
func Cwds(pids []int) map[int]string {
	res := make(map[int]string, len(pids))
	var mu sync.Mutex
	sem := make(chan struct{}, cwdWorkers)
	var wg sync.WaitGroup
	for _, pid := range pids {
		wg.Add(1)
		sem <- struct{}{}
		go func(pid int) {
			defer wg.Done()
			defer func() { <-sem }()
			cwd := Cwd(pid)
			mu.Lock()
			res[pid] = cwd
			mu.Unlock()
		}(pid)
	}
	wg.Wait()
	return res
}
