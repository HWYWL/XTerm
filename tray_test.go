package main

import (
	"bytes"
	"image/png"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo"
)

func TestTrayIcon(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(trayIcon()))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("size %v", b)
	}
	inked := 0
	for y := range 32 {
		for x := range 32 {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				inked++
			}
		}
	}
	if inked < 20 || inked > 32*32/2 {
		t.Errorf("%d pixels drawn", inked)
	}
}

// The tray's menu sets what closing the window does, with the answer the
// app remembers checked.
func TestTrayCloseActionMenu(t *testing.T) {
	old := updateMenuItem
	updateMenuItem = func() *mygo.MenuItem { return &mygo.MenuItem{Label: "updates"} }
	defer func() { updateMenuItem = old }()
	action := config.CloseTray
	p := &platform{
		closeAction:    func() string { return action },
		setCloseAction: func(a string) { action = a },
	}
	m := p.trayMenu()
	var sub *mygo.MenuItem
	for _, it := range m.Items() {
		if it.Label == locales.TrayCloseAction() {
			sub = it
		}
	}
	if sub == nil || len(sub.Submenu) != 3 {
		t.Fatalf("no close action menu: %+v", m.Items())
	}
	checked := map[string]bool{}
	for _, it := range sub.Submenu {
		checked[it.Label] = it.Checked
	}
	if !checked[locales.OptionCloseTray()] || checked[locales.OptionCloseAsk()] || checked[locales.OptionCloseQuit()] {
		t.Errorf("checked %v", checked)
	}

	// Choosing another remembers it.
	for _, it := range sub.Submenu {
		if it.Label == locales.OptionCloseQuit() {
			it.Click(it, nil)
		}
	}
	if action != config.CloseQuit {
		t.Errorf("action %q", action)
	}
}
