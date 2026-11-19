package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/opencode"
	"github.com/ubgo/agentio/pi"

	"github.com/thesatellite-ai/agentop/internal/model"
)

// Theme centralizes every color so the UI reads as one designed surface, not a
// pile of defaults.
var theme = struct {
	Base         tcell.Color // app background
	Accent       tcell.Color // primary brand accent
	Header       tcell.Color
	HeaderBG     tcell.Color
	Footer       tcell.Color
	Border       tcell.Color
	BorderActive tcell.Color
	Selected     tcell.Color
	SelectedBG   tcell.Color
	CursorBG     tcell.Color
	Muted        tcell.Color
	Text         tcell.Color
	Good         tcell.Color
	Warn         tcell.Color
	Danger       tcell.Color
	BarFill      tcell.Color
	BarTrack     tcell.Color
}{
	Base:         tcell.NewHexColor(0x1A1B26),
	Accent:       tcell.NewHexColor(0x7AA2F7),
	Header:       tcell.NewHexColor(0xC0CAF5),
	HeaderBG:     tcell.NewHexColor(0x1F2335),
	Footer:       tcell.NewHexColor(0x565F89),
	Border:       tcell.NewHexColor(0x3B4261),
	BorderActive: tcell.NewHexColor(0x7AA2F7),
	Selected:     tcell.NewHexColor(0x9ECE6A),
	SelectedBG:   tcell.NewHexColor(0x283457),
	CursorBG:     tcell.NewHexColor(0x2E3C64),
	Muted:        tcell.NewHexColor(0x565F89),
	Text:         tcell.NewHexColor(0xC0CAF5),
	Good:         tcell.NewHexColor(0x9ECE6A),
	Warn:         tcell.NewHexColor(0xE0AF68),
	Danger:       tcell.NewHexColor(0xF7768E),
	BarFill:      tcell.NewHexColor(0x7AA2F7),
	BarTrack:     tcell.NewHexColor(0x292E42),
}

// sourceColor gives each controller a stable, distinct hue.
func sourceColor(s model.Source) tcell.Color {
	switch s {
	case model.SourceEmdash:
		return tcell.NewHexColor(0x73DACA) // teal
	case model.SourceCmux:
		return tcell.NewHexColor(0xBB9AF7) // violet
	case model.SourceRunbay:
		return tcell.NewHexColor(0xFF007C) // magenta
	case model.SourceDirect:
		return tcell.NewHexColor(0xE0AF68) // amber
	default:
		return theme.Muted
	}
}

// agentColor gives each agent a stable, distinct hue, kept clear of the
// source colors above so AGENT and SRC columns never blur together. An agent
// without its own hue (a custom agentio agent) is muted.
func agentColor(a agentio.ID) tcell.Color {
	switch a {
	case claude.ID:
		return tcell.NewHexColor(0xFF9E64) // orange
	case codex.ID:
		return tcell.NewHexColor(0x7DCFFF) // cyan
	case gemini.ID:
		return tcell.NewHexColor(0xFCA7EA) // pink
	case pi.ID:
		return tcell.NewHexColor(0xC3E88D) // lime
	case opencode.ID:
		return tcell.NewHexColor(0xF7C67F) // sand
	default:
		return theme.Muted
	}
}

// idleColor escalates color with staleness so old sessions draw the eye.
func idleColor(s model.Session) tcell.Color {
	switch d := s.IdleDays(); {
	case d >= 3:
		return theme.Danger
	case d >= 1:
		return theme.Warn
	default:
		return theme.Good
	}
}

// hex returns a tview color tag like "[#7aa2f7]".
func hex(c tcell.Color) string {
	r, g, b := c.RGB()
	return "[#" + twoHex(r) + twoHex(g) + twoHex(b) + "]"
}

func twoHex(v int32) string {
	const d = "0123456789abcdef"
	return string([]byte{d[(v>>4)&0xf], d[v&0xf]})
}
