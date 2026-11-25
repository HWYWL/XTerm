package app

import (
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The window has no title bar of its own (mygo.WindowOptions.TitleBarStyle):
// the view draws one, so that the app's background shows behind the window's
// buttons too. MyGo tells where the buttons are
// (ui.Context.TitleBar): at the right on Windows and Linux, at its left, the
// traffic lights, on macOS. The row drags the window, and a double click
// maximizes it.

// titleBar draws the bar the window's buttons sit in, when the main window
// has no title bar of its own: nothing in a window that has one, or in full
// screen, where the buttons hide.
func (a *App) titleBar(c *ui.Context) { a.chromeBar(c, locales.AppTitle()) }

// chromeBar is the row of a window without a title bar of its own: its
// title, in the row that drags the window. Secondary windows draw it as the
// main one does (openWindow).
func (a *App) chromeBar(c *ui.Context, title string) {
	bar := c.TitleBar()
	if bar.Height <= 0 {
		return
	}
	ui.Row(c).Height(bar.Height).Padding(0, bar.Right, 0, bar.Left).DragWindow().
		AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			ui.Text(c, title).FontSize(12).TextColor(theme.TextMuted).SingleLine()
			ui.Spacer(c)
		})
}
