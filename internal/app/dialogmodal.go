package app

import (
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// A dialog comes in over dialogIn, and goes out over dialogOut.
const (
	dialogIn  = 140 * time.Millisecond
	dialogOut = 100 * time.Millisecond
)

// dialogRise is how far, in DIPs, a dialog rises as it comes in.
const dialogRise = 8

// The dialog's glass: these round its corners, and blur what it shows of
// what it dims.
const (
	dialogRadius = 18
	dialogBlur   = 30
)

// dialogGlass is the fill and the stroke of a dialog in t: a sheet of
// light over what it dims, in the night and in the day.
func dialogGlass(t *ui.Theme) (from, to, stroke ui.Color) {
	if t.Dark {
		return ui.RGBA(20, 26, 44, 0.72), ui.RGBA(16, 20, 34, 0.62), ui.RGBA(255, 255, 255, 0.115)
	}
	return ui.RGBA(255, 255, 255, 0.92), ui.RGBA(255, 255, 255, 0.80), ui.RGBA(255, 255, 255, 0.95)
}

// dialogMotion is how a dialog comes in, and goes out.
type dialogMotion struct {
	opened  time.Time // when it began to come in
	closing time.Time // when it began to go out, zero while it shows
}

// begin makes the dialog go out: it is gone dialogOut from now.
func (m *dialogMotion) begin(now time.Time) {
	if m.closing.IsZero() {
		m.closing = now
	}
}

// goingOut reports whether it goes out.
func (m *dialogMotion) goingOut() bool { return !m.closing.IsZero() }

// gone reports whether its way out is done.
func (m *dialogMotion) gone(now time.Time) bool {
	return m.goingOut() && now.Sub(m.closing) >= dialogOut
}

// fade reports how opaque the dialog is in this frame, from 0 to 1: it
// comes in as it opens, and goes out once it closes.
func (m *dialogMotion) fade(now time.Time) float32 {
	if m.goingOut() {
		return clamp01(1 - float32(now.Sub(m.closing))/float32(dialogOut))
	}
	if m.opened.IsZero() {
		m.opened = now
	}
	return clamp01(float32(now.Sub(m.opened)) / float32(dialogIn))
}

func clamp01(v float32) float32 { return max(0, min(v, 1)) }

// dialogModal shows the dialog fn builds over the window, as ui.Modal does,
// as a pane of glass that comes in and goes out as m says.
//
// ui.Modal's own panel cannot move: it sits on a backdrop that is new in
// every frame, so no Enter of a Transition plays for it, and it is gone in
// the frame it closes. The backdrop and the panel here are the same, with
// the glass look and the motion m gives: *open is cleared by Escape and by
// a click outside the panel, as before.
func (a *App) dialogModal(c *ui.Context, m *dialogMotion, open *bool, fn func()) {
	now := time.Now()
	p := m.fade(now)
	if p < 1 {
		c.Invalidate() // the next frame shows it further in, or out
	}
	winW, winH := a.plat.WindowSize(WindowMain)
	t := theme
	from, to, stroke := dialogGlass(t)
	ui.DialogBase(c, open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.42*p))
		panel.Padding(t.Space(6)).Gap(t.Space(3)).Radius(dialogRadius).
			Gradient(from, to, 180).
			Border(1, stroke).
			Shadow(0, 20, 50, -18, ui.RGBA(0, 0, 0, 0.72)).
			InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.16)).
			Material(glass.Blur{Radius: dialogBlur}).
			MaxWidth(float32(winW)-t.Space(10)).MaxHeight(float32(winH)-t.Space(10)).
			Opacity(p).
			TransformOrigin(0.5, 0.5).
			Translate(0, (1-p)*dialogRise).
			Scale(0.98+0.02*p, 0.98+0.02*p).
			Role(ui.RoleDialog)
		fn()
	})
}
