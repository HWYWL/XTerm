package app

import (
	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// closeAskView is the question the close button asks while the app does not
// know yet what closing means: quit, or keep running in the tray, with a
// box that remembers the answer for the next time. Escape, or a click
// outside it, cancels, and the window stays open.
func (a *App) closeAskView(c *ui.Context) {
	if !a.closeAsk {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, locales.CloseAskTitle()).FontSize(16).Bold()
			ui.Text(c, locales.CloseAskBody())
			ui.Checkbox(c, &a.closeAskRemember, locales.CloseAskRemember())
			widgets.Muted(c, locales.CloseAskNote()).FontSize(12)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, locales.Cancel()).Clicked() {
					open = false
				}
				// A system without a tray has nowhere to keep the app.
				if a.plat.HasTray() {
					if ui.Button(c, locales.CloseAskTray()).Clicked() {
						open = false
						a.closeChosen(config.CloseTray)
					}
				}
				if ui.PrimaryButton(c, locales.CloseAskQuit()).Clicked() {
					open = false
					a.closeChosen(config.CloseQuit)
				}
			})
		})
	})
	if !open {
		a.closeAsk = false
	}
}

// closeChosen does what the user answered the question with, remembering it
// when the box is checked: the tray keeps the app running, the window
// hidden; quit quits, asking about the sessions as it does.
func (a *App) closeChosen(action string) {
	remember := a.closeAskRemember
	a.closeAsk, a.closeAskRemember = false, false
	if remember {
		a.SetCloseAction(action)
	}
	if action == config.CloseTray && a.hideToTray() {
		return
	}
	a.plat.Quit()
}
