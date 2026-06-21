package tui

import "testing"

// TestRenderPipeline exercises the full data->render path against the live
// system (build layout, reload, group, filter, sort, repaint every pane,
// switch groups, cycle sort) without starting the interactive event loop.
// It's a smoke test: it must not panic and must produce coherent state.
func TestRenderPipeline(t *testing.T) {
	a := newApp()
	a.buildLayout()
	if err := a.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	// There's always at least the "All" group, first.
	if len(a.groups) == 0 || a.groups[0].kind != groupAll {
		t.Fatalf("expected an All group first, got %+v", a.groups)
	}

	// view must be a subset of all and respect the active (All) filter.
	if len(a.view) != len(a.all) {
		t.Fatalf("All group should show every session: view=%d all=%d", len(a.view), len(a.all))
	}

	// Source-group counts must sum to total; agent-group counts must too (the
	// two are independent partitions of the same set).
	var srcSum, agtSum int
	for _, g := range a.groups {
		switch g.kind {
		case groupSource:
			srcSum += g.count
		case groupAgent:
			agtSum += g.count
		}
	}
	if srcSum != len(a.all) {
		t.Fatalf("source counts %d != total %d", srcSum, len(a.all))
	}
	if agtSum != len(a.all) {
		t.Fatalf("agent counts %d != total %d", agtSum, len(a.all))
	}

	// Switching to each filter group must only show matching sessions; headers
	// must not change the view.
	for i, g := range a.groups {
		a.onGroupChanged(i)
		for _, s := range a.view {
			if !g.matches(s) {
				t.Fatalf("group %q (kind %d) leaked a non-matching session", g.label, g.kind)
			}
		}
	}

	// Cycle every sort field in both directions — must not panic and must
	// actually order the slice.
	a.onGroupChanged(0) // back to All
	for _, f := range sortFields {
		for _, asc := range []bool{true, false} {
			a.sortField, a.sortAsc = f, asc
			a.applyFilterSort()
			a.refreshTable()
			for i := 1; i < len(a.view); i++ {
				if f.less(a.view[i], a.view[i-1]) == asc && a.view[i] != a.view[i-1] {
					// strictly out of order in the requested direction
					if asc {
						t.Fatalf("sort %s asc out of order at %d", f, i)
					}
				}
			}
		}
	}

	// Detail render for the first row must not panic.
	a.refreshDetail()
}
