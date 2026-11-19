package discover

import (
	"os"
	"os/exec"
	"testing"
)

// TestLookupProcs reads (never signals) this test process and an exited child.
func TestLookupProcs(t *testing.T) {
	self := os.Getpid()
	child := exec.Command("true")
	if err := child.Run(); err != nil {
		t.Skipf("cannot run true: %v", err)
	}
	gone := child.Process.Pid

	rows, err := LookupProcs([]int{self, gone})
	if err != nil {
		t.Fatalf("LookupProcs: %v", err)
	}
	row, ok := rows[self]
	if !ok || row.StartedAt.IsZero() || row.PID != self {
		t.Fatalf("self row = %+v, ok=%v", row, ok)
	}
	if _, ok := rows[gone]; ok {
		t.Fatalf("exited child %d still listed", gone)
	}

	only, err := LookupProcs([]int{gone})
	if err != nil || len(only) != 0 {
		t.Fatalf("lookup of only ended pids = %v, %v; want empty, nil", only, err)
	}
	if empty, err := LookupProcs(nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty lookup = %v, %v", empty, err)
	}
}
