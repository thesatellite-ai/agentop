package tui

// Layout tokens — every TUI dimension lives here as a named constant so sizing
// is defined once and never appears as a bare literal at a use site. Values are
// in terminal cells (width) or rows (height) unless noted.
const (
	headerHeight = 1 // top status bar, one row
	footerHeight = 1 // bottom keybinding bar, one row

	sidebarWidth    = 26 // source sidebar column width
	detailHeight    = 9  // detail pane height (fixed; table takes the rest)
	tableProportion = 3  // table flex weight vs. the rest of the right column

	memBarWidth       = 10  // width of a sidebar group's memory bar
	agentColWidth     = 8   // AGENT column width
	taskColWidth      = 22  // TASK column width, shared by the table cell and its truncation
	commandPreviewLen = 240 // max chars of the command line shown in the detail pane

	// Modal sizes (centered overlays). Content height is computed from the
	// number of rows plus modalChromeRows for the border.
	sortModalWidth  = 28
	reapModalWidth  = 46
	reapModalHeight = 7
	reapFieldWidth  = 6 // input width for the reap-by-idle days field
	helpModalWidth  = 60
	modalChromeRows = 2 // border rows added around a modal's content
)

// focusPane identifies which pane currently owns the keyboard. Modeled as a
// typed enum (not bare 0/1) so Tab-cycling and focus checks are self-describing.
type focusPane int

const (
	focusSidebar focusPane = iota
	focusTable
	focusPaneCount // sentinel: number of focusable panes, for modulo cycling
)
