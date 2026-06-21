package tui

import "github.com/gdamore/tcell/v2"

func (a *App) wireKeys() {
	a.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if a.modalOpen || a.filtering {
			return ev // modal/filter input handles its own keys (incl. Esc/Enter)
		}
		switch ev.Key() {
		case tcell.KeyTab, tcell.KeyBacktab:
			a.setFocus((a.focusIdx + 1) % focusPaneCount)
			return nil
		case tcell.KeyEscape:
			// Esc from any pane clears an active quick-filter.
			if a.filterQuery != "" {
				a.clearFilter()
				return nil
			}
		case tcell.KeyCtrlC:
			a.app.Stop()
			return nil
		case tcell.KeyRune:
			switch ev.Rune() {
			case 'q':
				a.app.Stop()
				return nil
			case '?':
				a.openHelp()
				return nil
			case 's':
				a.openSortModal()
				return nil
			case 'i':
				a.openReapModal()
				return nil
			case '/':
				a.openFilter()
				return nil
			case 'r':
				_ = a.reload()
				a.setStatus("refreshed")
				return nil
			}
		}
		if a.focusIdx == focusSidebar {
			return a.sidebarKeys(ev)
		}
		return a.tableKeys(ev)
	})
}

func (a *App) sidebarKeys(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyRune {
		switch ev.Rune() {
		case 'j':
			a.moveSidebar(1)
			return nil
		case 'k':
			a.moveSidebar(-1)
			return nil
		}
	}
	return ev
}

func (a *App) tableKeys(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyDelete:
		a.killFlow()
		return nil
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'j':
			a.moveTable(1)
			return nil
		case 'k':
			a.moveTable(-1)
			return nil
		case 'g':
			a.gotoTable(1)
			return nil
		case 'G':
			a.gotoTable(len(a.view))
			return nil
		case ' ':
			a.toggleSelect()
			return nil
		case 'x':
			a.killFlow()
			return nil
		case 'a':
			a.selectAllView()
			return nil
		case 'c':
			a.clearSelect()
			return nil
		}
	}
	return ev
}

func (a *App) moveSidebar(d int) {
	n := a.sidebar.GetItemCount()
	if n == 0 {
		return
	}
	step := d
	if step == 0 {
		step = 1
	}
	i := a.sidebar.GetCurrentItem() + d
	// Skip non-selectable section headers, continuing in the travel direction.
	for i >= 0 && i < n && i < len(a.groups) && a.groups[i].kind == groupHeader {
		i += step
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	a.sidebar.SetCurrentItem(i)
}

func (a *App) moveTable(d int) {
	if len(a.view) == 0 {
		return
	}
	row, _ := a.table.GetSelection()
	row += d
	if row < 1 {
		row = 1
	}
	if row > len(a.view) {
		row = len(a.view)
	}
	a.table.Select(row, 0)
}

func (a *App) gotoTable(row int) {
	if len(a.view) == 0 {
		return
	}
	if row < 1 {
		row = 1
	}
	if row > len(a.view) {
		row = len(a.view)
	}
	a.table.Select(row, 0)
}

func (a *App) toggleSelect() {
	s, ok := a.currentSession()
	if !ok {
		return
	}
	if a.selected[s.PID] {
		delete(a.selected, s.PID)
	} else {
		a.selected[s.PID] = true
	}
	a.refreshTable()
	a.moveTable(1) // advance for fast multi-select
	a.setStatus("")
}

func (a *App) selectAllView() {
	for _, s := range a.view {
		a.selected[s.PID] = true
	}
	a.refreshTable()
	a.setStatus("selected all in view")
}

func (a *App) clearSelect() {
	a.selected = map[int]bool{}
	a.refreshTable()
	a.setStatus("selection cleared")
}

// selectedOrCursor returns the selected PIDs, or the cursor row if none.
func (a *App) selectedOrCursor() []int {
	var pids []int
	for _, s := range a.view {
		if a.selected[s.PID] {
			pids = append(pids, s.PID)
		}
	}
	if len(pids) == 0 {
		if s, ok := a.currentSession(); ok {
			pids = []int{s.PID}
		}
	}
	return pids
}
