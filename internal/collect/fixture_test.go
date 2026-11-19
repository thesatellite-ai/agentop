package collect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/builtin"
)

// Fixed clock and start times shared by the tests in this package.
var (
	t0  = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	t1  = t0.Add(time.Hour)
	now = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
)

// Conversation ids alice's and bob's sessions resume (hyphenated UUIDs, the
// shape agentio accepts as an argv join).
const (
	aliceConv = "0f79d084-6061-439e-b571-3668de33ed34"
	bobConv   = "5db26401-1f22-49eb-ab3f-447f44409c34"
)

// row builds a process row the way agentio's darwin reader fills one from a
// `ps` command column: Argv split on spaces, Exe = argv[0], CommandLine the
// raw string. Pure.
func row(pid, ppid int, start time.Time, command string) agentio.Process {
	argv := strings.Fields(command)
	p := agentio.Process{PID: pid, PPID: ppid, Start: start, Argv: argv, CommandLine: command}
	if len(argv) > 0 {
		p.Exe = argv[0]
	}
	return p
}

// testCollector returns a Collector on an agentio fixture host (temp user
// home, the given process table, fixed clock) with every built-in agent.
// Nothing in it reads or signals a real process.
func testCollector(t *testing.T, procs *agenttest.FakeProcesses) *Collector {
	t.Helper()
	h := agenttest.NewHost(t, agenttest.WithProcesses(procs), agenttest.WithNow(func() time.Time { return now }))
	agents, err := builtin.New(h)
	if err != nil {
		t.Fatal(err)
	}
	return &Collector{Host: h, Agents: agents.All()}
}

// pidFile is the part of Claude's sessions/<pid>.json the tests write.
type pidFile struct {
	PID             int    `json:"pid"`
	SessionID       string `json:"sessionId"`
	Cwd             string `json:"cwd"`
	StartedAt       int64  `json:"startedAt"`
	Version         string `json:"version"`
	Kind            string `json:"kind"`
	Entrypoint      string `json:"entrypoint"`
	Status          string `json:"status"`
	UpdatedAt       int64  `json:"updatedAt"`
	StatusUpdatedAt int64  `json:"statusUpdatedAt"`
}

// writePidFile writes <userHome>/.claude/sessions/<pid>.json the way Claude
// Code records a live session.
func writePidFile(t *testing.T, userHome string, f pidFile) {
	t.Helper()
	dir := filepath.Join(userHome, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(f.PID)+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
