package app

import "github.com/egoist/mygo/ui"

// The sky the window shows behind its glass: a deep blue at night and a
// soft blue-white by day, with the glows of QuickShell's aurora.
var (
	skyTop    = ui.Hex("#070a13")
	skyBottom = ui.Hex("#0b1224")
	dayTop    = ui.Hex("#e8f0ff")
	dayBottom = ui.Hex("#e2f6fb")
)

// aurora paints the backdrop of the window of kind, behind everything else
// it builds: the main window's, and every secondary window's, so that a
// dialog is the app's own color and not the system's.
func (a *App) aurora(c *ui.Context, kind WindowKind) {
	w, h := a.plat.WindowSize(kind)
	if w <= 0 || h <= 0 {
		return
	}
	W, H := float32(w), float32(h)
	if theme.Dark {
		ui.Box(c).Absolute().ZIndex(-1).Left(0).Top(0).Size(W, H).Gradient(skyTop, skyBottom, 160)
	} else {
		ui.Box(c).Absolute().ZIndex(-1).Left(0).Top(0).Size(W, H).Gradient(dayTop, dayBottom, 158)
	}
	glow := func(cx, cy, r float32, col ui.Color) {
		ui.Box(c).Absolute().ZIndex(-1).Left(cx-r).Top(cy-r).Size(2*r, 2*r).Radius(r).
			RadialGradient(ui.RadialGradient{Stops: []ui.GradientStop{
				{Color: col, At: 0}, {Color: ui.Transparent, At: 1},
			}})
	}
	if theme.Dark {
		glow(W*0.04, H*0.02, W*0.30, ui.RGBA(31, 110, 242, 0.24))
		glow(W*0.98, H*0.30, W*0.26, ui.RGBA(122, 60, 224, 0.20))
		glow(W*0.72, H*0.02, W*0.20, ui.RGBA(23, 184, 200, 0.18))
		glow(W*0.96, H*1.02, W*0.24, ui.RGBA(194, 58, 142, 0.16))
		return
	}
	glow(W*0.04, H*0.02, W*0.30, ui.RGBA(143, 186, 255, 0.45))
	glow(W*0.98, H*0.30, W*0.26, ui.RGBA(205, 176, 255, 0.40))
	glow(W*0.72, H*0.02, W*0.20, ui.RGBA(140, 227, 242, 0.42))
	glow(W*0.96, H*1.02, W*0.24, ui.RGBA(255, 185, 216, 0.38))
}
