package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The app opens its left column on the connections, to pick a host, and
// keeps the page chosen from then on.
func TestLeftPageOnOpen(t *testing.T) {
	t.Setenv("FINALLYSHELL_DIR", t.TempDir())
	p := newFakePlatform(t)
	if a := New(p); a.left.page != leftConnections {
		t.Errorf("page %d on open", a.left.page)
	}
	if err := config.SaveSettings(config.Settings{LeftPanelPage: config.LeftPanelSystem}); err != nil {
		t.Fatal(err)
	}
	if a := New(p); a.left.page != leftSystem {
		t.Errorf("page %d, saved system", a.left.page)
	}
}

// The left column's tabs show its pages one at a time: the server monitor,
// the session list and the connections. The page is kept, and the monitor's
// tab goes with it to the right column.
func TestLeftTabs(t *testing.T) {
	a, p := testApp(t)
	demoSession(a, "web-1", "45.56.78.90")
	a.active = 0
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()

	// The helper leaves it on the sessions, with the three tabs in the row.
	if a.left.page != leftSessions {
		t.Fatalf("page %d at the start", a.left.page)
	}
	var tabs []ui.Rect
	for _, want := range []string{locales.System(), locales.SessionsTab(), locales.ConnectionsPage()} {
		r, ok := tt.Find(want)
		if !ok {
			t.Errorf("no tab %q; texts %q", want, tt.Texts())
			continue
		}
		tabs = append(tabs, r)
	}
	// Each a pill of its own, with a gap to the next.
	for i := 1; i < len(tabs); i++ {
		if tabs[i].X < tabs[i-1].X+tabs[i-1].W+4 {
			t.Errorf("tabs %d and %d at %+v and %+v, no gap between", i-1, i, tabs[i-1], tabs[i])
		}
	}
	if !tt.HasText("web-1") || tt.HasText("IP 45.56.78.90") || tt.HasText(locales.SearchConnections()) {
		t.Errorf("the sessions page shows more than its list; texts %q", tt.Texts())
	}

	// The system tab shows the monitor alone.
	if err := tt.Click(locales.System()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.left.page != leftSystem || config.LoadSettings().LeftPanelPage != config.LeftPanelSystem {
		t.Errorf("page %d, saved %q", a.left.page, config.LoadSettings().LeftPanelPage)
	}
	if r := find(t, tt, "IP 45.56.78.90"); r.X > a.sideWidth {
		t.Errorf("the monitor at %+v, outside the left column", r)
	}
	if tt.HasText(locales.NoSessions()) {
		t.Errorf("the session list shows on the system page; texts %q", tt.Texts())
	}

	// The connections tab shows the tree, and its search.
	if err := tt.Click(locales.ConnectionsPage()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.left.page != leftConnections || config.LoadSettings().LeftPanelPage != config.LeftPanelConnections {
		t.Errorf("page %d, saved %q", a.left.page, config.LoadSettings().LeftPanelPage)
	}
	if !tt.HasText(locales.SearchConnections()) || tt.HasText("IP 45.56.78.90") {
		t.Errorf("connections page: texts %q", tt.Texts())
	}

	// Back to the sessions, where the buttons of the page are.
	if err := tt.Click(locales.SessionsTab()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.left.page != leftSessions || config.LoadSettings().LeftPanelPage != "" {
		t.Errorf("page %d, saved %q", a.left.page, config.LoadSettings().LeftPanelPage)
	}
	if _, ok := tt.Find(locales.NewTab()); !ok {
		t.Errorf("no new tab button; texts %q", tt.Texts())
	}

	// With the monitor in the right column, its tab goes, and the page with
	// it: the connections stay shown.
	a.setMonitorOnRight(true)
	tt.Frame()
	if _, ok := tt.Find(locales.System()); ok {
		t.Errorf("system tab with the monitor on the right; texts %q", tt.Texts())
	}
	a.setLeftPage(leftSystem)
	tt.Frame()
	if _, ok := tt.Find(locales.System()); ok {
		t.Errorf("system page shown with the monitor on the right; texts %q", tt.Texts())
	}
	a.setLeftPage(leftConnections)
	tt.Frame()
	if a.left.page != leftConnections || !tt.HasText(locales.SearchConnections()) {
		t.Errorf("page %d with the monitor on the right", a.left.page)
	}
}
