package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/thesatellite-ai/agentop/internal/model"
)

func mem(b uint64) string {
	const mb = 1024 * 1024
	v := float64(b) / mb
	if v >= 1024 {
		return fmt.Sprintf("%.1fGB", v/1024)
	}
	return fmt.Sprintf("%.0fMB", v)
}

func taskLabel(s model.Session) string {
	if s.Task == "" {
		return "—"
	}
	return s.Task
}

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

// bar renders a fixed-width proportional meter using block glyphs.
func bar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	fill := int(frac*float64(width) + 0.5)
	return strings.Repeat("█", fill) + strings.Repeat("░", width-fill)
}

// homePath replaces the user's home prefix with ~ for compact cwd display.
func homePath(p, home string) string {
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// since renders an absolute time as a compact "Jun 15 22:59" plus age.
func sinceLabel(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("Jan _2 15:04")
}
