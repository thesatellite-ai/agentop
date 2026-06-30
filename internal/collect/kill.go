package collect

import (
	"slices"
	"syscall"
	"time"
)

// Kill sends SIGTERM (graceful) to a PID. agentop never SIGKILLs by default —
// SIGTERM lets Claude flush; the conversation lives in the controller's store,
// not the process heap, so nothing is lost.
func Kill(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// Alive reports whether a PID currently exists. Signal 0 does the kernel's
// permission/existence check without delivering a signal: a nil error (ours) or
// EPERM (exists but owned by another user) both mean the process is still here;
// ESRCH means it is gone.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// KillAll terminates every PID, returning the first error encountered (but
// attempting all of them).
func KillAll(pids []int) error {
	var firstErr error
	for _, pid := range pids {
		if err := Kill(pid); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Reap terminates pids and waits for them to actually exit. It sends SIGTERM,
// then polls up to grace for each process to disappear — long enough for Claude
// to flush. Any survivor still alive after grace is escalated to SIGKILL (a
// retry that can't be ignored), since the caller asked for the process to be
// gone. Returns the pids still alive after escalation (normally empty).
//
// This is why a single keypress is enough: callers used to SIGTERM and reload
// immediately, before the async signal had taken effect, so the dying process
// reappeared and looked un-killed. Reap blocks until the kernel confirms death.
func Reap(pids []int, grace time.Duration) []int {
	if len(pids) == 0 {
		return nil
	}
	for _, p := range pids {
		_ = Kill(p) // SIGTERM — give it a chance to flush
	}

	const pollEvery = 50 * time.Millisecond
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !anyAlive(pids) {
			return nil
		}
		time.Sleep(pollEvery)
	}

	// Grace elapsed; force the survivors.
	for _, p := range pids {
		if Alive(p) {
			_ = syscall.Kill(p, syscall.SIGKILL)
		}
	}
	// SIGKILL is delivered asynchronously too — wait briefly for the reap.
	killDeadline := time.Now().Add(time.Second)
	for time.Now().Before(killDeadline) {
		if !anyAlive(pids) {
			return nil
		}
		time.Sleep(pollEvery)
	}

	var survivors []int
	for _, p := range pids {
		if Alive(p) {
			survivors = append(survivors, p)
		}
	}
	return survivors
}

func anyAlive(pids []int) bool {
	return slices.ContainsFunc(pids, Alive)
}
