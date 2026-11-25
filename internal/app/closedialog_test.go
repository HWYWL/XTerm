package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// closeTest opens the app's window to draw the question in.
func closeTest(t *testing.T) (*App, *fakePlatform, *ui.Tester) {
	t.Helper()
	a, p := testApp(t)
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	return a, p, tt
}

// The close button asks the first time, and the checked box remembers the
// answer: the next close keeps the app in the tray without a question.
func TestCloseAskRemembers(t *testing.T) {
	a, p, tt := closeTest(t)
	if a.CloseRequested() {
		t.Fatal("closed without asking")
	}
	p.drain() // the question comes with the next frame
	tt.Frame()
	if !a.closeAsk || !tt.HasText(locales.CloseAskTitle()) {
		t.Fatalf("no question; texts %q", tt.Texts())
	}
	if err := tt.Click(locales.CloseAskRemember()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.closeAskRemember {
		t.Fatal("the box did not check")
	}
	if err := tt.Click(locales.CloseAskTray()); err != nil {
		t.Fatal(err)
	}
	if a.closeAsk || a.settings.CloseAction != config.CloseTray || !p.inTray {
		t.Fatalf("tray: asking %v, action %q, in the tray %v", a.closeAsk, a.settings.CloseAction, p.inTray)
	}

	// The next close goes to the tray at once, with no question.
	p.inTray = false
	if a.CloseRequested() || !p.inTray || a.closeAsk {
		t.Fatalf("remembered: in the tray %v, asking %v", p.inTray, a.closeAsk)
	}

	// Asking again, as the tray's menu or the options do, asks once more.
	a.SetCloseAction(config.CloseAsk)
	p.inTray = false
	if a.CloseRequested() || p.inTray {
		t.Fatal("closed or hid")
	}
	p.drain()
	if !a.closeAsk {
		t.Error("did not ask again")
	}
}

// The question's Quit quits, and remembers it.
func TestCloseAskQuits(t *testing.T) {
	a, p, tt := closeTest(t)
	a.CloseRequested()
	p.drain()
	tt.Frame()
	if err := tt.Click(locales.CloseAskRemember()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(locales.CloseAskQuit()); err != nil {
		t.Fatal(err)
	}
	if a.closeAsk || a.settings.CloseAction != config.CloseQuit {
		t.Fatalf("asking %v, action %q", a.closeAsk, a.settings.CloseAction)
	}
	if p.quits != 1 {
		t.Fatalf("quit %d times", p.quits)
	}
	// Remembered: the next close quits with no question.
	if !a.CloseRequested() || a.closeAsk {
		t.Error("asked again")
	}
}

// A system without a tray has nowhere to keep the app: the question offers
// quitting alone.
func TestCloseAskWithoutTray(t *testing.T) {
	a, p, tt := closeTest(t)
	p.noTray = true
	a.CloseRequested()
	p.drain()
	tt.Frame()
	if tt.HasText(locales.CloseAskTray()) || !tt.HasText(locales.CloseAskQuit()) {
		t.Errorf("without a tray: texts %q", tt.Texts())
	}
}

// Escape leaves the window open, and asks again next time.
func TestCloseAskCancels(t *testing.T) {
	a, p, tt := closeTest(t)
	demoSession(a, "web", "10.0.0.1")
	a.CloseRequested()
	p.drain()
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.closeAsk || a.settings.CloseAction != config.CloseAsk || p.inTray || p.quits != 0 {
		t.Fatalf("cancel: asking %v, action %q, in the tray %v, quit %d",
			a.closeAsk, a.settings.CloseAction, p.inTray, p.quits)
	}
}
