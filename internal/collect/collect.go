// Package collect orchestrates the discovery pipeline:
// discover -> classify agent -> resolve cwd -> classify source -> enrich
// (emdash) -> compute idle -> rank.
package collect

import (
	"sort"
	"time"

	"github.com/thesatellite-ai/agentop/internal/agent"
	"github.com/thesatellite-ai/agentop/internal/discover"
	"github.com/thesatellite-ai/agentop/internal/emdash"
	"github.com/thesatellite-ai/agentop/internal/model"
	"github.com/thesatellite-ai/agentop/internal/source"
)

// Sessions runs the full pipeline and returns enriched, ranked sessions
// (most-idle and largest first).
func Sessions() ([]model.Session, error) {
	procs, err := discover.ListProcs()
	if err != nil {
		return nil, err
	}

	// Index every process by PID for parent-chain classification.
	index := make(map[int]discover.Proc, len(procs))
	for _, p := range procs {
		index[p.PID] = p
	}

	// Filter to agent processes.
	var agents []discover.Proc
	for _, p := range procs {
		if agent.IsAgent(p.Command) {
			agents = append(agents, p)
		}
	}

	// Resolve cwds in parallel.
	pids := make([]int, len(agents))
	for i, p := range agents {
		pids[i] = p.PID
	}
	cwds := discover.Cwds(pids)

	// emdash enrichment map (best-effort; empty on any error).
	emap, _ := emdash.LoadMap()

	now := time.Now()
	sessions := make([]model.Session, 0, len(agents))
	for _, p := range agents {
		cwd := cwds[p.PID]
		s := model.Session{
			PID:       p.PID,
			PPID:      p.PPID,
			Agent:     agent.Classify(p.Command),
			RSSBytes:  p.RSSBytes,
			Cwd:       cwd,
			Command:   p.Command,
			TTY:       p.TTY,
			TTYActive: discover.TTYActivity(p.TTY),
			StartedAt: p.StartedAt,
			Source:    source.Classify(p.PID, index, cwd),
		}
		if info, ok := emap[cwd]; ok {
			s.Task = info.Task
			s.TaskStatus = info.Status
			s.LastActive = info.LastActive
		}
		s.ComputeIdle(now)
		sessions = append(sessions, s)
	}

	Rank(sessions)
	return sessions, nil
}

// Rank sorts sessions by idle (desc) then memory (desc) so the best reap
// candidates float to the top.
func Rank(s []model.Session) {
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].Idle != s[j].Idle {
			return s[i].Idle > s[j].Idle
		}
		return s[i].RSSBytes > s[j].RSSBytes
	})
}
