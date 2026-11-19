// Package collect orchestrates the discovery pipeline on agentio: one
// process-table read -> classify agent (sessions only) -> resolve cwds -> one
// batch read of what each agent recorded -> classify launcher -> enrich
// (emdash) -> compute idle -> rank. It also owns the kill path (kill.go), which takes agentio's
// stop plan as input and sends the signals itself.
package collect

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"

	"github.com/thesatellite-ai/agentop/internal/discover"
	"github.com/thesatellite-ai/agentop/internal/emdash"
	"github.com/thesatellite-ai/agentop/internal/model"
	"github.com/thesatellite-ai/agentop/internal/source"
)

// cwdWorkers bounds the concurrent cwd lookups (an lsof spawn per pid on
// darwin). There can be 25+ agent processes, so they run in parallel.
const cwdWorkers = 12

// Collector reads agent sessions through agentio. Every input (process
// table, homes, clock, emdash DB, tty activity) is a field, so tests run it
// on a fixture host and never depend on what runs on the machine.
type Collector struct {
	// Host is the machine agentio reads: process table, cwd, env, homes.
	Host agentio.Host
	// Agents are the agents to recognise, in classification order.
	Agents []agentio.Agent
	// Emdash returns the emdash cwd -> task map (best effort).
	Emdash func() (map[string]emdash.Info, error)
	// TTYActivity returns the last I/O time of a controlling terminal.
	TTYActivity func(tty string) time.Time
}

// New returns a Collector on the real machine with every built-in agentio
// agent (Claude Code, Codex, Gemini CLI, pi, opencode).
func New() (*Collector, error) {
	h, err := agentio.HostFromOS()
	if err != nil {
		return nil, fmt.Errorf("collect: host: %w", err)
	}
	return NewWithHost(h)
}

// NewWithHost returns a Collector on h (a fixture host in tests) with every
// built-in agentio agent, the emdash DB and real terminal activity.
func NewWithHost(h agentio.Host) (*Collector, error) {
	agents, err := builtin.New(h)
	if err != nil {
		return nil, fmt.Errorf("collect: agents: %w", err)
	}
	return &Collector{
		Host:        h,
		Agents:      agents.All(),
		Emdash:      emdash.LoadMap,
		TTYActivity: discover.TTYActivity,
	}, nil
}

// Sessions runs the full pipeline and returns enriched, ranked sessions
// (most-idle and largest first).
func (c *Collector) Sessions(ctx context.Context) ([]model.Session, error) {
	table, err := c.table(ctx)
	if err != nil {
		return nil, err
	}

	// Keep agent conversation processes only. agentio classifies agent
	// servers, launchers and one-shot commands (`claude mcp serve`, `claude
	// mcp list`, `codex mcp …`, `codex app-server`, the npm codex launcher)
	// as non-session kinds: they are never listed, so they can never be
	// reaped.
	type row struct {
		p    agentio.Process
		info agentio.ProcessInfo
	}
	var rows []row
	for _, p := range table {
		if info, ok := agentio.ClassifyProcess(c.Agents, p); ok && info.Session {
			rows = append(rows, row{p, info})
		}
	}

	emap := map[string]emdash.Info{}
	if c.Emdash != nil {
		if m, err := c.Emdash(); err == nil {
			emap = m // best effort: an unreadable emdash DB only loses task names
		}
	}

	// Cwds in parallel (lsof per pid on darwin), stored on the rows so the
	// agent readers below never look them up again.
	var wg sync.WaitGroup
	sem := make(chan struct{}, cwdWorkers)
	for i := range rows {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if cwd, err := c.Host.Processes.Cwd(ctx, rows[i].p.PID); err == nil {
				rows[i].p.Cwd = cwd
			}
		}()
	}
	wg.Wait()

	// What each agent recorded about its processes, in one batch read per
	// agent. The join is best effort: a session the agent recorded nothing
	// about (or whose files cannot be read) is still listed without it, so a
	// failed read is dropped and the sessions found are kept.
	procs := make([]agentio.Process, len(rows))
	for i, r := range rows {
		procs[i] = r.p
	}
	joins, _ := agentio.FindProcessSessions(ctx, c.Agents, procs)

	now := c.now()
	sessions := make([]model.Session, len(rows))
	for i, r := range rows {
		ps, found := joins[r.p.PID]
		sessions[i] = c.session(r.p, r.info, ps, found, table, emap, now)
	}

	Rank(sessions)
	return sessions, nil
}

// session builds one Session from its process row (cwd already filled),
// what the agent recorded about the process (agentio ProcessSession, when
// found), launcher, emdash task and idle.
func (c *Collector) session(p agentio.Process, info agentio.ProcessInfo, ps agentio.ProcessSession, found bool, table []agentio.Process, emap map[string]emdash.Info, now time.Time) model.Session {
	s := model.Session{
		PID:       p.PID,
		PPID:      p.PPID,
		Agent:     info.Agent,
		Kind:      info.Kind,
		RSSBytes:  p.RSS,
		Cwd:       p.Cwd,
		Command:   p.CommandString(),
		TTY:       p.TTY,
		StartedAt: p.Start,
	}
	if c.TTYActivity != nil {
		s.TTYActive = c.TTYActivity(p.TTY)
	}
	if found {
		applyProcessSession(&s, ps)
	}
	s.Source = source.Classify(p, table, s.Cwd)
	if info, ok := emap[s.Cwd]; ok {
		s.Task = info.Task
		s.TaskStatus = info.Status
		s.LastActive = info.LastActive
	}
	s.ComputeIdle(now)
	return s
}

// applyProcessSession copies what the agent recorded onto s. The recorded
// directory fills the cwd only when the process cwd could not be read.
func applyProcessSession(s *model.Session, ps agentio.ProcessSession) {
	s.ConversationID = ps.ConversationID
	if !ps.Home.IsZero() {
		s.Home = ps.Home.Dir()
	}
	s.Version = ps.Version
	s.Status = ps.Status
	s.AgentActive = ps.LastActivity
	s.Join = ps.Confidence
	if s.Cwd == "" {
		s.Cwd = ps.Dir
	}
}

// table reads the process table once.
func (c *Collector) table(ctx context.Context) ([]agentio.Process, error) {
	if c.Host.Processes == nil {
		return nil, fmt.Errorf("collect: the host has no process table")
	}
	table, err := c.Host.Processes.Processes(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect: process table: %w", err)
	}
	return table, nil
}

// now reads the host clock, falling back to the real one.
func (c *Collector) now() time.Time {
	if c.Host.Now != nil {
		return c.Host.Now()
	}
	return time.Now()
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
