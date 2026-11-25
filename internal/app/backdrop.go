package app

import "github.com/egoist/mygo/ui"

// backdrop paints the background of the window of kind, behind everything
// else it builds: the main window's, and every secondary window's, in the
// theme's own background, so that a dialog is the app's color and not the
// system's.
func (a *App) backdrop(c *ui.Context, kind WindowKind) {
	w, h := a.plat.WindowSize(kind)
	if w <= 0 || h <= 0 {
		return
	}
	ui.Box(c).Absolute().ZIndex(-1).Left(0).Top(0).Size(float32(w), float32(h)).
		Background(theme.Background)
}
