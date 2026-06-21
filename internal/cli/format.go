package cli

import (
	"fmt"
	"strings"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// mem formats bytes as a compact MB/GB string.
func mem(b uint64) string {
	const mb = 1024 * 1024
	v := float64(b) / mb
	if v >= 1024 {
		return fmt.Sprintf("%.1fGB", v/1024)
	}
	return fmt.Sprintf("%.0fMB", v)
}

// taskLabel returns the emdash task name or a dash.
func taskLabel(s model.Session) string {
	if s.Task == "" {
		return "-"
	}
	return s.Task
}

// trunc shortens a string to n runes with an ellipsis.
func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// CLI table layout. The printf width specifiers in renderTable's format
// strings are kept in sync with these by hand (Go format strings can't
// interpolate constants); the values used as actual arguments — task
// truncation and the rule width — reference the constants directly.
const (
	listTaskColWidth = 26  // TASK column width
	listRuleWidth    = 100 // width of the header underline rule
)

// rowFormat aligns one data/header row: PID AGENT MEM SRC IDLE TASK CWD.
// The width specifiers are kept in sync with listTaskColWidth by hand.
const rowFormat = "%-7s %-7s %-8s %-7s %-8s %-26s %s\n"

// renderTable prints sessions as an aligned text table for the `list` and
// `reap` commands. The trailing legend documents the `*` proxy marker.
func renderTable(sessions []model.Session) string {
	var b strings.Builder
	fmt.Fprintf(&b, rowFormat, "PID", "AGENT", "MEM", "SRC", "IDLE", "TASK", "CWD")
	b.WriteString(strings.Repeat("─", listRuleWidth))
	b.WriteByte('\n')
	var total uint64
	for _, s := range sessions {
		total += s.RSSBytes
		fmt.Fprintf(&b, rowFormat,
			fmt.Sprintf("%d", s.PID), string(s.Agent), mem(s.RSSBytes), string(s.Source), s.IdleLabel(),
			trunc(taskLabel(s), listTaskColWidth), s.Cwd)
	}
	fmt.Fprintf(&b, "\n%d sessions · %s total · * = idle from process age, not last activity\n",
		len(sessions), mem(total))
	return b.String()
}
