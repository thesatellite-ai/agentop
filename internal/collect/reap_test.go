package collect

import (
	"os/exec"
	"testing"
	"time"
)

// TestReapEscalates spawns a child that ignores SIGTERM and confirms Reap
// falls back to SIGKILL within grace. Run: go test ./internal/collect/ -run Reap -v
func TestReapEscalates(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go cmd.Wait() // reap zombie after SIGKILL

	if !Alive(pid) {
		t.Fatal("child should be alive")
	}
	start := time.Now()
	survivors := Reap([]int{pid}, 500*time.Millisecond) // short grace -> forces SIGKILL path
	if len(survivors) != 0 {
		t.Fatalf("SIGKILL escalation failed, survivors=%v", survivors)
	}
	if Alive(pid) {
		t.Fatal("pid still alive after Reap")
	}
	t.Logf("reaped SIGTERM-ignoring pid %d in %s via SIGKILL", pid, time.Since(start))
}
