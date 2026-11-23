package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// TestSidebarHideAndShow: the button at the left end of the row of session
// tabs hides the whole left side, the row and the terminal take the width,
// and the button shows it again.
func TestSidebarHideAndShow(t *testing.T) {
	a, p := testApp(t)
	demoSession(a, "web-1", "10.0.0.1")
	tt := ui.NewTester(a.View, winW, winH)
	p.tt = tt
	tt.Frame()

	tab, _ := tabBox(t, tt, "web-1")
	if tab.X <= a.sideWidth {
		t.Fatalf("the row of tabs at %v, not beside the sidebar (%v)", tab.X, a.sideWidth)
	}

	if err := tt.Click(locales.HideSidebar()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.sideHidden || !config.LoadSettings().SidebarHidden {
		t.Fatalf("hidden %v, saved %v", a.sideHidden, config.LoadSettings().SidebarHidden)
	}
	if tab, _ = tabBox(t, tt, "web-1"); tab.X > 40 {
		t.Errorf("the row of tabs at %v with the sidebar hidden", tab.X)
	}
	if _, ok := tt.Find(locales.ShowSidebar()); !ok {
		t.Errorf("no Show sidebar; texts %q", tt.Texts())
	}

	if err := tt.Click(locales.ShowSidebar()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.sideHidden || config.LoadSettings().SidebarHidden {
		t.Fatalf("hidden %v, saved %v after Show sidebar", a.sideHidden, config.LoadSettings().SidebarHidden)
	}
	if tab, _ = tabBox(t, tt, "web-1"); tab.X <= a.sideWidth {
		t.Errorf("the row of tabs at %v, not back beside the sidebar (%v)", tab.X, a.sideWidth)
	}
}

// TestSidebarMenuAndShortcut: the row of tabs' context menu and the
// shortcut hide the left side too.
func TestSidebarMenuAndShortcut(t *testing.T) {
	a, p := testApp(t)
	demoSession(a, "web-1", "10.0.0.1")
	tt := ui.NewTester(a.View, winW, winH)
	p.tt = tt
	tt.Frame()

	// A right click on the row, past the tabs and the +.
	tt.RightClickAt(winW-30, sessionTabBarHeight/2)
	if err := tt.ChooseMenuItem(locales.HideSidebar()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if !a.sideHidden {
		t.Fatal("the menu did not hide the sidebar")
	}

	k := a.shortcut(scSidebar)
	tt.Key(k.Mods, k.Key)
	tt.Frame()
	if a.sideHidden {
		t.Fatal("the shortcut did not show the sidebar")
	}
	tt.Key(k.Mods, k.Key)
	tt.Frame()
	if !a.sideHidden {
		t.Fatal("the shortcut did not hide the sidebar")
	}
}

// TestSidebarHiddenRestored: the setting is read back at the next start.
func TestSidebarHiddenRestored(t *testing.T) {
	a, p := testApp(t)
	a.setSidebarHidden(true)
	b := New(p)
	if !b.sideHidden {
		t.Error("not read back")
	}
	tt := ui.NewTester(b.View, winW, winH)
	tt.Frame()
	if _, ok := tt.Find(locales.ShowSidebar()); !ok {
		t.Errorf("no Show sidebar at the next start; texts %q", tt.Texts())
	}
}
