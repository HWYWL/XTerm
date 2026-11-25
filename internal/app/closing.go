package app

import (
	"strings"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// Confirmation is a question with two answers, which the platform asks in
// a message box of the system's.
type Confirmation struct {
	Title, Message, Detail string
	// OK is the button that goes ahead, Cancel the one that does not, which
	// Escape and closing the box answer too.
	OK, Cancel string
}

// The main window's close button and quitting, as the General options and
// the tray's menu say (Settings.CloseAction):
//
//	                    close button                 Quit (tray menu, Cmd+Q, ...)
//	ask (the first time)  asks: quit, or the tray     ask if sessions are open*
//	the tray            hide to the tray             ask if sessions are open*
//	quit                ask if sessions are open*,   ask if sessions are open*,
//	                    then quit                    then quit
//
// * unless "Confirm before closing" is off. A system without a tray (Linux
// without AppIndicator) closes as with quit.

// CloseAction is what closing the main window does now: config.CloseAsk
// while the app has not been told, else the answer it remembers.
func (a *App) CloseAction() string {
	switch a.settings.CloseAction {
	case config.CloseTray, config.CloseQuit:
		return a.settings.CloseAction
	}
	return config.CloseAsk
}

// SetCloseAction remembers what closing the window does: config.CloseAsk to
// ask again, config.CloseTray to keep running in the tray, or
// config.CloseQuit to quit. The tray's menu and the options set it.
func (a *App) SetCloseAction(action string) {
	switch action {
	case config.CloseTray, config.CloseQuit:
	default:
		action = config.CloseAsk
	}
	if a.CloseAction() == action {
		return
	}
	a.changeSettings(func() { a.settings.CloseAction = action })
}

// CloseRequested tells whether the main window may close now, as the user
// asks with its close button: the platform calls it on the UI thread, and
// keeps the window open when it returns false. The app then has hidden
// the window to the tray, asked what to do, or asked whether to quit, and
// quits itself if the user says yes.
func (a *App) CloseRequested() bool {
	if a.quitApproved {
		return true // the quit closes the windows
	}
	switch a.CloseAction() {
	case config.CloseTray:
		if a.hideToTray() {
			return false
		}
		return a.QuitRequested()
	case config.CloseQuit:
		return a.QuitRequested()
	}
	a.askClose()
	return false
}

// hideToTray hides the window behind the tray icon, closing the other
// windows with it. It reports false, leaving the window as it is, when the
// system has no tray.
func (a *App) hideToTray() bool {
	if !a.plat.HideToTray() {
		return false
	}
	a.closeWindows()
	return true
}

// askClose shows the question over the main window, in the next frame:
// quitting, or keeping the app in the tray, and whether to remember the
// answer (closedialog.go).
func (a *App) askClose() {
	if a.closeAsk {
		return
	}
	a.post(func() {
		if a.closeAsk || a.quitApproved {
			return
		}
		a.closeAsk, a.closeAskRemember = true, false
		a.redraw()
	})
}

// QuitRequested tells whether the app may quit now, as the tray's Quit,
// the system's (Cmd+Q on macOS) or the last window closing asks: the
// platform calls it on the UI thread before the quit closes any window.
// With sessions open it asks first, unless the options say not to, and
// returns false; the app quits itself if the user says yes. Files with
// changes in the editor it asks about whatever the options say, as the
// changes would be lost.
func (a *App) QuitRequested() bool {
	if a.quitApproved {
		return true
	}
	n, unsaved := a.liveSessions(), a.unsavedFiles()
	if n == 0 && unsaved == 0 || a.settings.NoConfirmClose && unsaved == 0 {
		a.quitApproved = true
		return true
	}
	if !a.askingQuit {
		a.askingQuit = true
		var detail []string
		if remote, local := a.localQuitCounts(); n > 0 && !a.settings.NoConfirmClose {
			if remote > 0 {
				detail = append(detail, locales.ConfirmQuitBody(remote))
			}
			if local > 0 {
				detail = append(detail, locales.ConfirmQuitLocal(local))
			}
		}
		if unsaved > 0 {
			detail = append(detail, locales.ConfirmQuitUnsaved(unsaved))
		}
		q := Confirmation{
			Title:   locales.AppTitle(),
			Message: locales.ConfirmQuitTitle(),
			Detail:  strings.Join(detail, "\n"),
			OK:      locales.ConfirmQuitOK(),
			Cancel:  locales.Cancel(),
		}
		go func() {
			yes := a.plat.Confirm(q)
			a.post(func() {
				a.askingQuit = false
				if yes {
					a.quitApproved = true
					a.plat.Quit()
				}
			})
		}()
	}
	return false
}

// liveSessions counts the sessions that are connected or connecting,
// which quitting ends: local terminals whose program runs too.
func (a *App) liveSessions() int {
	n := 0
	for _, s := range a.sessions {
		if s.status == statusConnected || s.status == statusConnecting {
			n++
		}
	}
	return n
}
