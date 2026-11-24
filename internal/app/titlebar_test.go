package app

import (
	"testing"

	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// A window with no title bar of its own has one drawn by the view, with the
// app's title in it, clear of the window's buttons, and the panes start
// under it. A window with its own title bar has none of it.
func TestTitleBar(t *testing.T) {
	a, p := testApp(t)
	demoSession(a, "web-1", "45.56.78.90")
	a.active = 0
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.SetTitleBar(ui.TitleBar{Height: 32, Right: 138})
	tt.Frame()

	title, ok := tt.Find(locales.AppTitle())
	if !ok {
		t.Fatalf("no title in the bar; texts %q", tt.Texts())
	}
	if title.Y < 0 || title.Y+title.H > 32 {
		t.Errorf("title at %+v, outside the bar", title)
	}
	if title.X+title.W > winW-138 {
		t.Errorf("title at %+v, under the window's buttons", title)
	}
	// The panes start under the bar.
	if tab := find(t, tt, locales.System()); tab.Y < 32 {
		t.Errorf("the left column at %+v, over the title bar", tab)
	}
	if row := find(t, tt, "web-1"); row.Y < 32 {
		t.Errorf("the row of tabs at %+v, over the title bar", row)
	}

	// Full screen, and windows with a title bar of their own, draw none.
	tt.SetTitleBar(ui.TitleBar{})
	tt.Frame()
	if _, ok := tt.Find(locales.AppTitle()); ok {
		t.Errorf("title in a window with its own title bar; texts %q", tt.Texts())
	}
}
