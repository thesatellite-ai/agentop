package collect

import (
	"testing"
	"time"

	"github.com/thesatellite-ai/agentop/internal/model"
)

func TestRank(t *testing.T) {
	s := []model.Session{
		{PID: 1, Idle: time.Hour, RSSBytes: 100},
		{PID: 2, Idle: 5 * time.Hour, RSSBytes: 50},
		{PID: 3, Idle: 5 * time.Hour, RSSBytes: 900}, // ties idle with #2, more mem
		{PID: 4, Idle: 0, RSSBytes: 999},
	}
	Rank(s)
	// Expected: most-idle first; ties broken by larger memory.
	wantOrder := []int{3, 2, 1, 4}
	for i, pid := range wantOrder {
		if s[i].PID != pid {
			t.Fatalf("position %d = pid %d, want %d (order %v)", i, s[i].PID, pid,
				[]int{s[0].PID, s[1].PID, s[2].PID, s[3].PID})
		}
	}
}
