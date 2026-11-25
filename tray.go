package main

import (
	"bytes"
	_ "embed"
	"image/color"
	"image/png"
	"log"
	"runtime"

	"finallyshell/internal/app"
	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

// ensureTray creates the tray icon when it is not there, and returns it: it
// is there all the while the app runs, so that its menu shows the window
// again, sets what closing does, or quits. It returns nil, leaving the
// window as it is, when the system has no tray (Linux without
// libayatana-appindicator3), where there would be no way back to the window.
func (p *platform) ensureTray() *mygo.Tray {
	if p.tray != nil {
		return p.tray
	}
	if p.trayErr {
		return nil
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "main",
		Icon:           trayIcon(),
		IconIsTemplate: runtime.GOOS != "linux",
		ToolTip:        locales.AppTitle(),
		Menu:           p.trayMenu(),
	})
	if err != nil {
		log.Println("tray:", err)
		p.trayErr = true
		return nil
	}
	tray.OnClick(p.showMain) // a double click on Linux, where a click opens the menu
	p.tray = tray
	return tray
}

// updateMenuItem is the tray menu's Check for Updates…, replaced by tests:
// the updater is used only once the app runs.
var updateMenuItem = updater.MenuItem

// trayMenu is the tray's menu: the window, the updates, what closing the
// window does, and quitting.
func (p *platform) trayMenu() *mygo.Menu {
	action := ""
	if p.closeAction != nil {
		action = p.closeAction()
	}
	choose := func(choice string) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			if p.setCloseAction != nil {
				p.setCloseAction(choice)
			}
			p.refreshTrayMenu()
		}
	}
	radio := func(choice, label string) *mygo.MenuItem {
		return &mygo.MenuItem{
			Type:    mygo.MenuItemRadio,
			Label:   label,
			Checked: action == choice,
			Click:   choose(choice),
		}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayShow(), Click: func(*mygo.MenuItem, *mygo.Window) { p.showMain() }},
		updateMenuItem(), // its window says why when this copy cannot update
		mygo.Separator(),
		{Label: locales.TrayCloseAction(), Type: mygo.MenuItemSubmenu, Submenu: []*mygo.MenuItem{
			radio(config.CloseAsk, locales.OptionCloseAsk()),
			radio(config.CloseTray, locales.OptionCloseTray()),
			radio(config.CloseQuit, locales.OptionCloseQuit()),
		}},
		mygo.Separator(),
		{Label: locales.TrayQuit(), Click: func(*mygo.MenuItem, *mygo.Window) { p.Quit() }},
	})
}

// refreshTrayMenu shows the tray's menu again, with the answer the app now
// remembers checked.
func (p *platform) refreshTrayMenu() {
	if p.tray != nil {
		p.tray.SetMenu(p.trayMenu())
	}
}

// HideToTray hides the main window behind the tray icon. It returns false,
// leaving the window as it is, when the system has no tray.
func (p *platform) HideToTray() bool {
	if p.ensureTray() == nil {
		return false
	}
	p.win.Hide()
	return true
}

// HasTray tells whether the system has a tray to keep the app in.
func (p *platform) HasTray() bool { return p.ensureTray() != nil }

// showMain shows the main window, out of the tray too. On macOS the Dock
// icon stays, and clicking it shows the window too (OnActivate in main).
func (p *platform) showMain() {
	p.win.Show()
	p.win.Restore()
	p.win.Focus()
	mygo.App.Focus()
}

// raise brings the main window to the front, as another launch of the
// app asks: out of the tray, or out of the Dock or taskbar when it is
// minimized. MyGo runs it with the right to take the focus that the launch
// passed on: its activation token on Wayland, AllowSetForegroundWindow on
// Windows.
func (p *platform) raise() {
	p.showMain()
}

// destroyTray takes the tray icon away, as the app quits.
func (p *platform) destroyTray() {
	if p.tray != nil {
		p.tray.Destroy()
		p.tray = nil
	}
}

func (p *platform) Confirm(q app.Confirmation) bool {
	var parent *mygo.Window
	if p.win.IsVisible() {
		parent = p.win // a sheet of it on macOS
	}
	r, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:        parent,
		Type:          mygo.MessageQuestion,
		Title:         q.Title,
		Message:       q.Message,
		Detail:        q.Detail,
		Buttons:       []string{q.OK, q.Cancel},
		DefaultButton: 0,
		CancelButton:  1,
	})
	if err != nil {
		// No message box: go ahead, rather than never let the app quit.
		log.Println("confirm:", err)
		return true
	}
	return r.Button == 0
}

// Quit quits as the system's Quit does: the app is asked first
// (OnBeforeQuit in main), then every window closes.
func (p *platform) Quit() { mygo.App.Quit() }

// traySVG is the app icon (docs/icon.svg) in one color, drawn in
// currentColor.
//
//go:embed tray.svg
var traySVG []byte

// trayIcon is tray.svg as a 32x32 PNG: black, for the system to tint, on
// macOS and Windows; blue on Linux, whose panels show it as it is, light
// or dark.
func trayIcon() []byte {
	var ink color.Color = color.Black
	if runtime.GOOS == "linux" {
		ink = color.RGBA{0x25, 0x63, 0xeb, 0xff}
	}
	var buf bytes.Buffer
	png.Encode(&buf, ui.MustParseSVG(traySVG).Image(32, 32, ink))
	return buf.Bytes()
}
