package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/thesatellite-ai/agentop/internal/collect"
	"github.com/thesatellite-ai/agentop/internal/model"
)

// Modal button labels — named so the label passed to AddButtons and the label
// compared in the done-callback can never drift apart.
const (
	btnCancel = "Cancel"
	btnKill   = "Kill"
)

// centered wraps a primitive in a fixed-size box centered on screen.
func centered(p tview.Primitive, w, h int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, h, 0, true).
			AddItem(nil, 0, 1, false), w, 0, true).
		AddItem(nil, 0, 1, false)
}

func (a *App) openModal(name string, p tview.Primitive, w, h int) {
	a.modalOpen = true
	a.pages.AddPage(name, centered(p, w, h), true, true)
	a.app.SetFocus(p)
}

func (a *App) closeModal(name string) {
	a.pages.RemovePage(name)
	a.modalOpen = false
	a.setFocus(a.focusIdx)
}

// scopeLabel describes what the TUI reap will act on: the active sidebar group
// plus the quick-filter, if any. reap operates on the current view, so the
// label must spell out both axes (e.g. "emdash · /sync").
func (a *App) scopeLabel() string {
	base := "all"
	if a.activeGroup.kind != groupAll {
		base = a.activeGroup.label
	}
	if a.filterQuery != "" {
		base += " · /" + a.filterQuery
	}
	return base
}

// --- sort picker -----------------------------------------------------------

func (a *App) openSortModal() {
	list := tview.NewList().ShowSecondaryText(false).
		SetMainTextColor(theme.Text).
		SetSelectedBackgroundColor(theme.SelectedBG).
		SetSelectedTextColor(tcell.ColorWhite).
		SetHighlightFullLine(true)
	list.SetBackgroundColor(theme.HeaderBG).
		SetBorder(true)
	list.SetTitle(" sort by ").SetTitleAlign(tview.AlignLeft).
		SetBorderColor(theme.BorderActive).SetTitleColor(theme.Header)

	for _, f := range sortFields {
		marker := "  "
		if f == a.sortField {
			if a.sortAsc {
				marker = "▲ "
			} else {
				marker = "▼ "
			}
		}
		field := f
		list.AddItem(marker+field.String(), "", 0, func() {
			a.chooseSort(field)
		})
		if f == a.sortField {
			list.SetCurrentItem(list.GetItemCount() - 1)
		}
	}
	list.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyEscape, ev.Rune() == 'q', ev.Rune() == 's':
			a.closeModal("sort")
			return nil
		case ev.Rune() == 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case ev.Rune() == 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}
		return ev
	})
	a.openModal("sort", list, sortModalWidth, len(sortFields)+modalChromeRows)
}

func (a *App) chooseSort(f SortField) {
	if f == a.sortField {
		a.sortAsc = !a.sortAsc // toggle direction
	} else {
		a.sortField = f
		// sensible default direction: idle & memory show biggest first
		a.sortAsc = !(f == SortIdle || f == SortMem)
	}
	a.applyFilterSort()
	a.refreshTable()
	a.closeModal("sort")
	dir := "desc"
	if a.sortAsc {
		dir = "asc"
	}
	a.setStatus(fmt.Sprintf("sorted by %s %s", a.sortField, dir))
}

// --- kill confirm ----------------------------------------------------------

func (a *App) killFlow() {
	pids := a.selectedOrCursor()
	if len(pids) == 0 {
		return
	}
	a.openKillModal(pids)
}

func (a *App) openKillModal(pids []int) {
	var total uint64
	pset := map[int]bool{}
	for _, p := range pids {
		pset[p] = true
	}
	for _, s := range a.all {
		if pset[s.PID] {
			total += s.RSSBytes
		}
	}
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Kill %d session(s) with SIGTERM,\nreclaim ~%s?", len(pids), mem(total))).
		AddButtons([]string{btnCancel, btnKill}).
		SetDoneFunc(func(idx int, label string) {
			if label == btnKill {
				a.doKill(pids)
			}
			a.closeModal("kill")
		})
	modal.SetBackgroundColor(theme.HeaderBG)
	a.modalOpen = true
	a.pages.AddPage("kill", modal, true, true)
	a.app.SetFocus(modal)
}

func (a *App) doKill(pids []int) {
	_ = collect.KillAll(pids)
	for _, p := range pids {
		delete(a.selected, p)
	}
	_ = a.reload()
	a.setStatus(fmt.Sprintf("reaped %d session(s)", len(pids)))
}

// --- reap by idle ----------------------------------------------------------

func (a *App) openReapModal() {
	input := tview.NewInputField().
		SetLabel("idle ≥ (days)  ").
		SetText(strconv.Itoa(model.DefaultReapIdleDays)).
		SetFieldWidth(reapFieldWidth).
		SetAcceptanceFunc(tview.InputFieldInteger)

	form := tview.NewForm().
		AddFormItem(input).
		AddButton("Reap", func() { a.reapByIdle(input.GetText()) }).
		AddButton("Cancel", func() { a.closeModal("reap") })
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetLabelColor(theme.Header).
		SetFieldBackgroundColor(theme.Base).
		SetFieldTextColor(theme.Text).
		SetButtonBackgroundColor(theme.Border).
		SetButtonTextColor(theme.Text)
	form.SetBackgroundColor(theme.HeaderBG)
	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" reap idle · scope: %s ", a.scopeLabel())).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(theme.BorderActive).SetTitleColor(theme.Header)
	form.SetCancelFunc(func() { a.closeModal("reap") })

	a.openModal("reap", form, reapModalWidth, reapModalHeight)
}

func (a *App) reapByIdle(text string) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 0 {
		a.setStatus("invalid idle threshold")
		a.closeModal("reap")
		return
	}
	var pids []int
	for _, s := range a.view { // a.view already reflects the sidebar group + quick-filter
		if s.IdleDays() >= n {
			pids = append(pids, s.PID)
		}
	}
	a.closeModal("reap")
	if len(pids) == 0 {
		a.setStatus(fmt.Sprintf("nothing idle ≥ %dd in %s", n, a.scopeLabel()))
		return
	}
	a.openKillModal(pids)
}

// --- help ------------------------------------------------------------------

func (a *App) openHelp() {
	tv := tview.NewTextView().SetDynamicColors(true)
	tv.SetTextColor(theme.Text).SetBackgroundColor(theme.HeaderBG)
	tv.SetBorder(true).SetTitle(" help ").SetTitleAlign(tview.AlignLeft).
		SetBorderColor(theme.BorderActive).SetTitleColor(theme.Header)
	ac := hex(theme.Accent)
	mu := hex(theme.Muted)
	lines := []string{
		fmt.Sprintf(" %sTab[-]        switch focus (sidebar ⇄ table)", ac),
		fmt.Sprintf(" %sj / k[-]      move down / up        %s↑ ↓ also work[-]", ac, mu),
		fmt.Sprintf(" %sg / G[-]      jump to top / bottom", ac),
		fmt.Sprintf(" %sspace[-]      select / deselect row", ac),
		fmt.Sprintf(" %sa / c[-]      select all / clear selection", ac),
		fmt.Sprintf(" %sx / Del[-]    kill selected (or cursor), SIGTERM", ac),
		fmt.Sprintf(" %si[-]          reap by idle; acts on the current view (group + filter)", ac),
		fmt.Sprintf(" %s/[-]          fuzzy filter (agent/src/task/cwd); Esc clears", ac),
		fmt.Sprintf(" %ss[-]          sort picker (toggle dir on same field)", ac),
		fmt.Sprintf(" %sr[-]          refresh", ac),
		fmt.Sprintf(" %sq[-]          quit", ac),
		"",
		fmt.Sprintf(" %sidle = time since last terminal I/O (real activity)[-]", mu),
		fmt.Sprintf(" %s*[-] = no controlling tty, fell back to process age (rare)", mu),
		"",
		fmt.Sprintf(" %spress any key to close[-]", mu),
	}
	tv.SetText(strings.Join(lines, "\n"))
	tv.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		a.closeModal("help")
		return nil
	})
	a.openModal("help", tv, helpModalWidth, len(lines)+modalChromeRows)
}
