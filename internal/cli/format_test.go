package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/thesatellite-ai/agentop/internal/model"
)

func TestRenderTable(t *testing.T) {
	sessions := []model.Session{
		{PID: 97170, Agent: model.AgentCodex, Source: model.SourceDirect, RSSBytes: 214 * 1024 * 1024, Idle: 2 * time.Minute, Cwd: "/code/agentop"},
		{PID: 45930, Agent: model.AgentClaude, Source: model.SourceEmdash, RSSBytes: 864 * 1024 * 1024, Task: "filemark", Cwd: "/code/fm"},
	}
	out := renderTable(sessions)

	// Header columns present, in order.
	for _, h := range []string{"PID", "AGENT", "MEM", "SRC", "IDLE", "TASK", "CWD"} {
		if !strings.Contains(out, h) {
			t.Errorf("header missing %q", h)
		}
	}
	// Row content present.
	for _, want := range []string{"97170", "codex", "direct", "45930", "claude", "emdash", "filemark"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
	// Summary line counts sessions and totals memory.
	if !strings.Contains(out, "2 sessions") {
		t.Error("missing session count in summary")
	}
	if !strings.Contains(out, "GB") {
		t.Error("expected a GB total in summary")
	}
}

func TestRenderTableEmpty(t *testing.T) {
	out := renderTable(nil)
	if !strings.Contains(out, "0 sessions") {
		t.Errorf("empty render should report 0 sessions, got: %q", out)
	}
}
