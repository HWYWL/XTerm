package app

import (
	"fmt"
	"slices"
	"testing"

	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// tabBox is the box of the session tab of name in the bar above the
// terminal, and whether it is the selected one.
func tabBox(t *testing.T, tt *ui.Tester, name string) (ui.Rect, bool) {
	t.Helper()
	info, ok := tt.AccessByRole(ui.RoleTab, name)
	if !ok {
		t.Fatalf("no tab %q; texts %q", name, tt.Texts())
	}
	return info.Bounds, info.Selected
}

// buttonBox is the box of the button labeled name.
func buttonBox(t *testing.T, tt *ui.Tester, name string) ui.Rect {
	t.Helper()
	info, ok := tt.AccessByRole(ui.RoleButton, name)
	if !ok {
		t.Fatalf("no button %q; texts %q", name, tt.Texts())
	}
	return info.Bounds
}

// tabsApp is the window with sessions named names, selected one selected,
// and the tester showing it.
func tabsApp(t *testing.T, selected int, names ...string) (*App, *ui.Tester) {
	t.Helper()
	a, p := testApp(t)
	for _, name := range names {
		demoSession(a, name, name+".example")
	}
	a.active = selected
	tt := ui.NewTester(a.View, winW, winH)
	p.tt = tt
	tt.Frame()
	return a, tt
}

func TestSessionTabsChooseSession(t *testing.T) {
	a, tt := tabsApp(t, 0, "alpha", "bravo")

	alpha, selected := tabBox(t, tt, "alpha")
	if !selected {
		t.Error("alpha's tab is not the selected one")
	}
	if alpha.Y+alpha.H > sessionTabBarHeight {
		t.Errorf("alpha's tab at %+v, not in the bar above the terminal", alpha)
	}
	if _, selected := tabBox(t, tt, "bravo"); selected {
		t.Error("bravo's tab is selected too")
	}

	bravo, _ := tabBox(t, tt, "bravo")
	tt.ClickAt(bravo.X+bravo.W/2, bravo.Y+bravo.H/2)
	tt.Frame()
	if a.active != 1 {
		t.Fatalf("a click on bravo's tab selected session %d", a.active)
	}
	if _, selected := tabBox(t, tt, "bravo"); !selected {
		t.Error("bravo's tab is not the selected one after the click")
	}
	if _, selected := tabBox(t, tt, "alpha"); selected {
		t.Error("alpha's tab is still selected")
	}
}

func TestSessionTabsCloseSession(t *testing.T) {
	a, tt := tabsApp(t, 1, "alpha", "bravo")

	bravo, _ := tabBox(t, tt, "bravo")
	// The × at the end of the selected tab.
	tt.ClickAt(bravo.X+bravo.W-8-9, bravo.Y+bravo.H/2)
	tt.Frame()
	if len(a.sessions) != 1 || a.sessions[0].label() != "alpha" {
		t.Fatalf("closed %q, sessions left %d", "bravo", len(a.sessions))
	}
	if a.active != 0 {
		t.Errorf("the selected session is %d after the close", a.active)
	}
	if _, ok := tt.AccessByRole(ui.RoleTab, "bravo"); ok {
		t.Error("bravo's tab is still in the bar")
	}
}

func TestSessionTabsNoPlusWithoutTabs(t *testing.T) {
	_, tt := tabsApp(t, -1)

	if _, ok := tt.AccessByRole(ui.RoleButton, locales.NewTabBeside()); ok {
		t.Error("the + shows with no tab open")
	}
}

func TestSessionTabsPlusBesideLastTab(t *testing.T) {
	a, p := testApp(t)
	alpha := demoSession(a, "alpha", "127.0.0.1")
	alpha.Host.Port = 1
	a.passwords[alpha.Host.ID] = "pw" // no prompt
	a.NewTab()                        // the last tab has no connection: the + takes alpha's
	tt := ui.NewTester(a.View, winW, winH)
	p.tt = tt
	tt.Frame()

	last, _ := tabBox(t, tt, locales.NewTab())
	plus := buttonBox(t, tt, locales.NewTabBeside())
	if plus.X < last.X+last.W {
		t.Errorf("the + at %v, not after the last tab, to %v", plus.X, last.X+last.W)
	}
	tt.ClickAt(plus.X+plus.W/2, plus.Y+plus.H/2)
	tt.Frame()
	if len(a.sessions) != 3 {
		t.Fatalf("%d sessions after the +", len(a.sessions))
	}
	dup := a.sessions[2]
	if dup.isNewTab() || dup.Host != alpha.Host {
		t.Errorf("the + opened %+v, not a tab for alpha's connection", dup)
	}
}

func TestSessionTabMenu(t *testing.T) {
	a, tt := tabsApp(t, 0, "alpha", "bravo", "charlie")

	alpha, _ := tabBox(t, tt, "alpha")
	tt.RightClickAt(alpha.X+alpha.W/2, alpha.Y+alpha.H/2)
	// A "-" is a separator.
	want := []string{
		locales.DuplicateTab(),
		"-",
		locales.Connect(), locales.ConnectAll(),
		"-",
		locales.Disconnect(),
		"-",
		locales.Close(),
		"-",
		locales.CloseOtherTabs(), locales.CloseAllTabs(),
	}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("the tab's menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem(locales.CloseOtherTabs()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.sessions) != 1 || a.sessions[0] != a.sessions[a.active] || a.sessions[0].label() != "alpha" {
		t.Fatalf("closed the others, sessions left %d, selected %d", len(a.sessions), a.active)
	}

	alpha, _ = tabBox(t, tt, "alpha")
	tt.RightClickAt(alpha.X+alpha.W/2, alpha.Y+alpha.H/2)
	if err := tt.ChooseMenuItem(locales.CloseAllTabs()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.sessions) != 0 {
		t.Fatalf("%d sessions after Close all", len(a.sessions))
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, locales.NewTabBeside()); ok {
		t.Error("the + shows with no tab open")
	}
}

// TestSessionTabMenuConnect: a tab whose session failed connects again
// from its menu.
func TestSessionTabMenuConnect(t *testing.T) {
	a, tt := tabsApp(t, 0, "alpha", "bravo")
	a.sessions[1].status, a.sessions[1].err = statusFailed, fmt.Errorf("refused")
	a.sessions[1].Host.Addr, a.sessions[1].Host.Port = "127.0.0.1", 1
	a.passwords[a.sessions[1].Host.ID] = "pw" // no prompt
	tt.Frame()

	bravo, _ := tabBox(t, tt, "bravo")
	tt.RightClickAt(bravo.X+bravo.W/2, bravo.Y+bravo.H/2)
	if err := tt.ChooseMenuItem(locales.Connect()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.sessions[1].status != statusConnecting {
		t.Errorf("bravo is %v after Connect, not connecting", a.sessions[1].status)
	}
}

func TestSessionTabsScrollSideways(t *testing.T) {
	var names []string
	for i := range 12 {
		names = append(names, fmt.Sprintf("server-%02d.example.com", i))
	}
	a, tt := tabsApp(t, 0, names...)

	if a.tabsScroll.MaxX <= 0 {
		t.Fatalf("12 tabs fit in the bar: nothing to scroll")
	}
	first, _ := tabBox(t, tt, names[0])
	tt.Wheel(first.X+first.W/2, sessionTabBarHeight/2, 0, 120)
	tt.Frame()
	if a.tabsScroll.X <= 0 {
		t.Error("the wheel did not scroll the tabs sideways")
	}
	if moved, _ := tabBox(t, tt, names[0]); moved.X >= first.X {
		t.Errorf("the first tab stayed at %v, from %v", moved.X, first.X)
	}
}
