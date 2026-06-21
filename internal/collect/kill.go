package collect

import "syscall"

// Kill sends SIGTERM (graceful) to a PID. agentop never SIGKILLs by default —
// SIGTERM lets Claude flush; the conversation lives in the controller's store,
// not the process heap, so nothing is lost.
func Kill(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
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
