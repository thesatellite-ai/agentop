// Package tui renders the interactive master-detail dashboard (tview/tcell):
// a filter sidebar (two axes — source and agent — each with counts and memory
// bars), a sortable process table filtered to the selected entry, and a detail
// pane that follows the cursor.
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/thesatellite-ai/agentop/internal/collect"
	"github.com/thesatellite-ai/agentop/internal/discover"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// groupKind tags what a sidebar entry filters by. The sidebar mixes two
// filter dimensions (source and agent) plus an "All" reset and non-selectable
// section headers, so each entry must declare which it is.
type groupKind int

const (
	groupAll    groupKind = iota // no filter — every session
	groupHeader                  // a non-filtering section label ("sources" / "agents")
	groupSource                  // filter by s.Source
	groupAgent                   // filter by s.Agent
)

// group is one sidebar entry.
type group struct {
	label string
	kind  groupKind
	src   model.Source    // set when kind == groupSource
	agent model.AgentKind // set when kind == groupAgent
	count int
	mem   uint64
}

// matches reports whether a session belongs in this group's filtered view.
func (g group) matches(s model.Session) bool {
	switch g.kind {
	case groupSource:
		return s.Source == g.src
	case groupAgent:
		return s.Agent == g.agent
	default: // groupAll / groupHeader (header keeps the prior view)
		return true
	}
}

// App is the whole TUI.
type App struct {
	app   *tview.Application
	pages *tview.Pages

	header      *tview.TextView
	sidebar     *tview.List
	table       *tview.Table
	detail      *tview.TextView
	footer      *tview.TextView
	filterInput *tview.InputField
	bottom      *tview.Flex // bottom row: footer, swapped for filterInput while filtering
	body        *tview.Flex

	all    []model.Session
	view   []model.Session
	groups []group

	activeGroup group  // the currently selected sidebar filter
	filterQuery string // active fuzzy quick-filter (empty = none)
	sortField   SortField
	sortAsc     bool
	selected    map[int]bool

	home    string
	physMem uint64
	status  string

	focusIdx   focusPane
	modalOpen  bool
	filtering  bool // true while the quick-filter input has focus
	populating bool // guards sidebar repopulation from firing onGroupChanged
}

// Run builds and runs the dashboard.
func Run() error {
	a := newApp()
	a.buildLayout()
	if err := a.reload(); err != nil {
		return err
	}
	a.wireKeys()
	a.setFocus(focusTable) // start on the table
	return a.app.SetRoot(a.pages, true).EnableMouse(true).Run()
}

// newApp constructs the App with all primitives but does not build the layout
// or start the event loop (so tests can exercise the data/render paths).
func newApp() *App {
	home, _ := os.UserHomeDir()
	return &App{
		app:         tview.NewApplication(),
		pages:       tview.NewPages(),
		header:      tview.NewTextView().SetDynamicColors(true),
		sidebar:     tview.NewList(),
		table:       tview.NewTable(),
		detail:      tview.NewTextView().SetDynamicColors(true).SetWrap(true),
		footer:      tview.NewTextView().SetDynamicColors(true),
		filterInput: tview.NewInputField(),
		activeGroup: group{label: "All", kind: groupAll},
		sortField:   SortIdle,
		sortAsc:     false, // most-idle first
		selected:    map[int]bool{},
		home:        home,
		physMem:     discover.PhysicalMemory(),
	}
}

func (a *App) buildLayout() {
	a.header.SetTextColor(theme.Header).SetBackgroundColor(theme.HeaderBG)

	a.sidebar.ShowSecondaryText(true).
		SetMainTextColor(theme.Text).
		SetSecondaryTextColor(theme.Muted).
		SetSelectedTextColor(tcell.ColorWhite).
		SetSelectedBackgroundColor(theme.SelectedBG).
		SetHighlightFullLine(true)
	a.sidebar.SetBackgroundColor(theme.Base).
		SetBorder(true)
	a.sidebar.SetTitle(" filters ").SetTitleAlign(tview.AlignLeft).
		SetBorderColor(theme.Border).SetTitleColor(theme.Muted)
	a.sidebar.SetChangedFunc(func(i int, _, _ string, _ rune) {
		a.onGroupChanged(i)
	})

	a.table.SetBorders(false).SetSelectable(true, false).SetFixed(1, 0)
	a.table.SetSelectedStyle(tcell.StyleDefault.Background(theme.CursorBG).Foreground(tcell.ColorWhite))
	a.table.SetBackgroundColor(theme.Base)
	a.table.SetBorder(true).SetTitle(" processes ").SetTitleAlign(tview.AlignLeft).
		SetBorderColor(theme.Border).SetTitleColor(theme.Muted)
	a.table.SetSelectionChangedFunc(func(row, col int) { a.refreshDetail() })

	a.detail.SetTextColor(theme.Text).SetBackgroundColor(theme.Base)
	a.detail.SetBorder(true).SetTitle(" detail ").SetTitleAlign(tview.AlignLeft).
		SetBorderColor(theme.Border).SetTitleColor(theme.Muted)

	a.footer.SetBackgroundColor(theme.HeaderBG)

	// Quick-filter input — lives in the bottom row, shown only while filtering.
	a.filterInput.SetLabel(" / ").
		SetLabelColor(theme.Accent).
		SetFieldBackgroundColor(theme.HeaderBG).
		SetFieldTextColor(theme.Text)
	a.filterInput.SetChangedFunc(a.onFilterChanged)
	a.filterInput.SetDoneFunc(a.onFilterDone)

	// bottom row holds the footer by default; openFilter swaps in filterInput.
	a.bottom = tview.NewFlex().AddItem(a.footer, 0, 1, false)
	a.bottom.SetBackgroundColor(theme.HeaderBG)

	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.table, 0, tableProportion, true).
		AddItem(a.detail, detailHeight, 0, false)
	right.SetBackgroundColor(theme.Base)

	a.body = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.sidebar, sidebarWidth, 0, true).
		AddItem(right, 0, 1, false)
	a.body.SetBackgroundColor(theme.Base)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, headerHeight, 0, false).
		AddItem(a.body, 0, 1, true).
		AddItem(a.bottom, footerHeight, 0, false)
	root.SetBackgroundColor(theme.Base)

	a.pages.SetBackgroundColor(theme.Base)
	a.pages.AddPage("main", root, true, true)
	a.refreshFooter()
}

// reload re-runs the discovery pipeline and repaints every pane.
func (a *App) reload() error {
	sessions, err := collect.Sessions()
	if err != nil {
		return err
	}
	a.all = sessions
	a.computeGroups()
	a.applyFilterSort()
	a.refreshHeader()
	a.refreshSidebar()
	a.refreshTable()
	a.refreshDetail()
	return nil
}

// sourceOrder / agentOrder fix the sidebar ordering so the list is stable
// across refreshes regardless of map iteration order.
var sourceOrder = []model.Source{model.SourceEmdash, model.SourceCmux, model.SourceDirect}
var agentOrder = []model.AgentKind{model.AgentClaude, model.AgentCodex}

// computeGroups rebuilds the sidebar entries: an "All" reset, then a Sources
// section and an Agents section (each preceded by a header), listing only the
// dimensions actually present. Sessions are counted in BOTH their source and
// their agent group — the two are independent filter axes over the same set.
func (a *App) computeGroups() {
	var total uint64
	srcCount, agtCount := map[model.Source]int{}, map[model.AgentKind]int{}
	srcMem, agtMem := map[model.Source]uint64{}, map[model.AgentKind]uint64{}
	for _, s := range a.all {
		srcCount[s.Source]++
		srcMem[s.Source] += s.RSSBytes
		agtCount[s.Agent]++
		agtMem[s.Agent] += s.RSSBytes
		total += s.RSSBytes
	}

	a.groups = []group{{label: "All", kind: groupAll, count: len(a.all), mem: total}}

	srcSection := false
	for _, src := range sourceOrder {
		if srcCount[src] == 0 {
			continue
		}
		if !srcSection {
			a.groups = append(a.groups, group{label: "sources", kind: groupHeader})
			srcSection = true
		}
		a.groups = append(a.groups, group{label: string(src), kind: groupSource, src: src, count: srcCount[src], mem: srcMem[src]})
	}

	agtSection := false
	for _, ag := range agentOrder {
		if agtCount[ag] == 0 {
			continue
		}
		if !agtSection {
			a.groups = append(a.groups, group{label: "agents", kind: groupHeader})
			agtSection = true
		}
		a.groups = append(a.groups, group{label: string(ag), kind: groupAgent, agent: ag, count: agtCount[ag], mem: agtMem[ag]})
	}
}

func (a *App) applyFilterSort() {
	q := strings.ToLower(strings.TrimSpace(a.filterQuery))
	a.view = a.view[:0]
	for _, s := range a.all {
		if !a.activeGroup.matches(s) {
			continue
		}
		if q != "" && !fuzzyMatch(q, sessionHaystack(s)) {
			continue
		}
		a.view = append(a.view, s)
	}
	sortSessions(a.view, a.sortField, a.sortAsc)
}

func (a *App) refreshHeader() {
	var total uint64
	for _, s := range a.all {
		total += s.RSSBytes
	}
	memPart := ""
	if a.physMem > 0 {
		pct := float64(total) / float64(a.physMem) * 100
		memPart = fmt.Sprintf(" · %s%.0f%% of %s[-]", hex(theme.Warn), pct, mem(a.physMem))
	}
	a.header.SetText(fmt.Sprintf(" %sagentop[-]  %s%d agents[-] · %s%s[-]%s",
		hex(theme.Accent), hex(theme.Header), len(a.all), hex(theme.Header), mem(total), memPart))
}

// groupColor returns the hue for a sidebar entry: accent for All, the source
// or agent hue for filters, muted for section headers.
func groupColor(g group) tcell.Color {
	switch g.kind {
	case groupSource:
		return sourceColor(g.src)
	case groupAgent:
		return agentColor(g.agent)
	case groupHeader:
		return theme.Muted
	default:
		return theme.Accent
	}
}

func (a *App) refreshSidebar() {
	cur := a.sidebar.GetCurrentItem()
	a.populating = true
	a.sidebar.Clear()
	// Scale memory bars to the largest filter group (skip All, which is the
	// total and would dwarf the rest, and headers which have no memory).
	var maxMem uint64 = 1
	for _, g := range a.groups {
		if g.kind == groupSource || g.kind == groupAgent {
			if g.mem > maxMem {
				maxMem = g.mem
			}
		}
	}
	for _, g := range a.groups {
		col := groupColor(g)
		if g.kind == groupHeader {
			// dim, indent-free section label; no count or bar
			a.sidebar.AddItem(fmt.Sprintf("%s── %s ──[-]", hex(theme.Muted), g.label), "", 0, nil)
			continue
		}
		main := fmt.Sprintf("%s%-8s[-] %s%2d[-]", hex(col), g.label, hex(theme.Muted), g.count)
		sec := fmt.Sprintf(" %s%s[-] %s%s[-]", hex(col), bar(float64(g.mem)/float64(maxMem), memBarWidth), hex(theme.Muted), mem(g.mem))
		a.sidebar.AddItem(main, sec, 0, nil)
	}
	if cur >= 0 && cur < a.sidebar.GetItemCount() {
		a.sidebar.SetCurrentItem(cur)
	}
	a.populating = false
}

func (a *App) onGroupChanged(i int) {
	if a.populating || i < 0 || i >= len(a.groups) {
		return
	}
	g := a.groups[i]
	if g.kind == groupHeader {
		return // headers don't filter — keep the current view
	}
	a.activeGroup = g
	a.applyFilterSort()
	a.refreshTable()
	a.refreshDetail()
}

type colSpec struct {
	title    string
	field    SortField
	sortable bool
	width    int // 0 = expand
}

func (a *App) columns() []colSpec {
	return []colSpec{
		{"", SortIdle, false, 2},
		{"PID", SortPID, true, 7},
		{"AGENT", SortAgent, true, agentColWidth},
		{"MEM", SortMem, true, 8},
		{"SRC", SortSource, true, 8},
		{"IDLE", SortIdle, true, 7},
		{"TASK", SortTask, true, taskColWidth},
		{"CWD", SortCwd, true, 0},
	}
}

func (a *App) refreshTable() {
	prevRow, _ := a.table.GetSelection()
	a.table.Clear()
	for c, col := range a.columns() {
		title := col.title
		if col.sortable && col.field == a.sortField {
			if a.sortAsc {
				title += " ▲"
			} else {
				title += " ▼"
			}
		}
		cell := tview.NewTableCell(title).
			SetTextColor(theme.Accent).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold)
		if col.width > 0 {
			cell.SetMaxWidth(col.width)
		}
		a.table.SetCell(0, c, cell)
	}
	for r, s := range a.view {
		mark := " "
		markColor := theme.Muted
		if a.selected[s.PID] {
			mark = "✓"
			markColor = theme.Selected
		}
		cells := []*tview.TableCell{
			tview.NewTableCell(mark).SetTextColor(markColor),
			tview.NewTableCell(fmt.Sprintf("%d", s.PID)).SetTextColor(theme.Muted),
			tview.NewTableCell(string(s.Agent)).SetTextColor(agentColor(s.Agent)),
			tview.NewTableCell(mem(s.RSSBytes)).SetTextColor(theme.Text),
			tview.NewTableCell(string(s.Source)).SetTextColor(sourceColor(s.Source)),
			tview.NewTableCell(s.IdleLabel()).SetTextColor(idleColor(s)),
			tview.NewTableCell(trunc(taskLabel(s), taskColWidth)).SetTextColor(taskColor(s)),
			tview.NewTableCell(homePath(s.Cwd, a.home)).SetTextColor(theme.Muted),
		}
		for c, cell := range cells {
			a.table.SetCell(r+1, c, cell)
		}
	}
	if len(a.view) > 0 {
		row := prevRow
		if row < 1 {
			row = 1
		}
		if row > len(a.view) {
			row = len(a.view)
		}
		a.table.Select(row, 0)
	}
	a.updateTableTitle()
}

// updateTableTitle reflects the active quick-filter (query + match count) in the
// table's border title so a filtered view is never mistaken for the full list.
func (a *App) updateTableTitle() {
	if a.filterQuery != "" {
		a.table.SetTitle(fmt.Sprintf(" processes · /%s (%d) ", a.filterQuery, len(a.view)))
		return
	}
	a.table.SetTitle(" processes ")
}

func taskColor(s model.Session) tcell.Color {
	if s.Task == "" {
		return theme.Muted
	}
	return theme.Text
}

func (a *App) refreshDetail() {
	s, ok := a.currentSession()
	if !ok {
		a.detail.SetText(fmt.Sprintf(" %sno session selected[-]", hex(theme.Muted)))
		return
	}
	lbl := func(k string) string { return fmt.Sprintf("%s%-9s[-]", hex(theme.Muted), k) }
	task := taskLabel(s)
	if s.TaskStatus != "" {
		task += fmt.Sprintf(" (%s)", s.TaskStatus)
	}
	active := sinceLabel(s.LastSeen())
	if s.IdleProxy {
		active = fmt.Sprintf("%s(no tty — using process age)[-]", hex(theme.Warn))
	}
	tty := s.TTY
	if tty == "" || tty == "??" {
		tty = "—"
	}
	var b strings.Builder
	fmt.Fprintf(&b, " %s pid %s%d[-]  %sppid %d[-]  %s%s[-]  %s%s[-]  %stty %s[-]\n",
		lbl("process"), hex(theme.Text), s.PID, hex(theme.Muted), s.PPID, hex(sourceColor(s.Source)), s.Source, hex(agentColor(s.Agent)), s.Agent, hex(theme.Muted), tty)
	fmt.Fprintf(&b, " %s %s%s[-]    %s %s%s[-]    %slast %s[-]\n",
		lbl("memory"), hex(theme.Text), mem(s.RSSBytes), lbl("idle"), hex(idleColor(s)), s.IdleLabel(), hex(theme.Muted), active)
	fmt.Fprintf(&b, " %s %s    %s %s\n", lbl("task"), task, lbl("started"), sinceLabel(s.StartedAt))
	fmt.Fprintf(&b, " %s %s%s[-]\n", lbl("cwd"), hex(theme.Text), homePath(s.Cwd, a.home))
	fmt.Fprintf(&b, " %s %s%s[-]", lbl("command"), hex(theme.Muted), trunc(firstLine(s.Command), commandPreviewLen))
	a.detail.SetText(b.String())
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (a *App) refreshFooter() {
	keys := []string{
		hex(theme.Accent) + "Tab[-] focus",
		hex(theme.Accent) + "j/k[-] move",
		hex(theme.Accent) + "space[-] select",
		hex(theme.Accent) + "x[-] kill",
		hex(theme.Accent) + "i[-] reap-idle",
		hex(theme.Accent) + "/[-] filter",
		hex(theme.Accent) + "s[-] sort",
		hex(theme.Accent) + "r[-] refresh",
		hex(theme.Accent) + "?[-] help",
		hex(theme.Accent) + "q[-] quit",
	}
	line := " " + strings.Join(keys, "  ")
	if a.status != "" {
		line = fmt.Sprintf(" %s%s[-]  ·%s", hex(theme.Good), a.status, line)
	}
	a.footer.SetText(line)
}

func (a *App) setStatus(msg string) {
	a.status = msg
	a.refreshFooter()
}

// currentSession returns the session under the table cursor.
func (a *App) currentSession() (model.Session, bool) {
	row, _ := a.table.GetSelection()
	idx := row - 1
	if idx < 0 || idx >= len(a.view) {
		return model.Session{}, false
	}
	return a.view[idx], true
}

func (a *App) setFocus(idx focusPane) {
	a.focusIdx = idx
	a.sidebar.SetBorderColor(theme.Border)
	a.table.SetBorderColor(theme.Border)
	switch idx {
	case focusSidebar:
		a.sidebar.SetBorderColor(theme.BorderActive)
		a.app.SetFocus(a.sidebar)
	default:
		a.table.SetBorderColor(theme.BorderActive)
		a.app.SetFocus(a.table)
	}
}

// --- quick filter ----------------------------------------------------------

// openFilter swaps the footer for the filter input and focuses it. While the
// input has focus, global keybindings are bypassed (see wireKeys) so the user
// can type freely.
func (a *App) openFilter() {
	a.filtering = true
	a.bottom.RemoveItem(a.footer)
	a.bottom.AddItem(a.filterInput, 0, 1, true)
	a.app.SetFocus(a.filterInput)
}

// closeFilter restores the footer and returns focus to the table. The query
// itself is left intact (the table stays filtered); onFilterDone clears it when
// the user cancels with Esc.
func (a *App) closeFilter() {
	a.filtering = false
	a.bottom.RemoveItem(a.filterInput)
	a.bottom.AddItem(a.footer, 0, 1, false)
	a.setFocus(focusTable)
}

// onFilterChanged re-filters live on every keystroke.
func (a *App) onFilterChanged(text string) {
	a.filterQuery = text
	a.applyFilterSort()
	a.refreshTable()
	a.refreshDetail()
}

// onFilterDone handles Enter (keep the filter) and Esc (clear it).
func (a *App) onFilterDone(key tcell.Key) {
	if key == tcell.KeyEscape {
		a.clearFilter()
	}
	a.closeFilter()
}

// clearFilter drops the active quick-filter and repaints. Safe to call when no
// filter is set. Used both by Esc inside the input and Esc from the table.
func (a *App) clearFilter() {
	if a.filterQuery == "" {
		return
	}
	a.filterQuery = ""
	a.filterInput.SetText("")
	a.applyFilterSort()
	a.refreshTable()
	a.refreshDetail()
}
