package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// A dialog comes in over dialogIn, and goes out over dialogOut.
const (
	dialogIn  = 140 * time.Millisecond
	dialogOut = 100 * time.Millisecond
)

// dialogRise is how far, in DIPs, a dialog rises as it comes in.
const dialogRise = 8

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

// dialogModal shows the dialog fn builds over the window, with the look of
// ui.Modal's panel, coming in and going out as m says.
//
// ui.Modal's own panel cannot move: it sits on a backdrop that is new in
// every frame, so no Enter of a Transition plays for it, and it is gone in
// the frame it closes. The backdrop and the panel here are the same, with
// a Transition of their own for the motion m gives: *open is cleared by
// Escape and by a click outside the panel, as before.
func (a *App) dialogModal(c *ui.Context, m *dialogMotion, open *bool, fn func()) {
	now := time.Now()
	p := m.fade(now)
	if p < 1 {
		c.Invalidate() // the next frame shows it further in, or out
	}
	winW, winH := a.plat.WindowSize(WindowMain)
	t := theme
	ui.DialogBase(c, open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.4*p))
		panel.Padding(t.Space(5)).Gap(t.Space(3)).Radius(t.Space(2.5)).
			Background(t.Background).
			MaxWidth(float32(winW)-t.Space(10)).MaxHeight(float32(winH)-t.Space(10)).
			Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3)).
			Opacity(p).
			TransformOrigin(0.5, 0.5).
			Translate(0, (1-p)*dialogRise).
			Scale(0.98+0.02*p, 0.98+0.02*p).
			Role(ui.RoleDialog)
		fn()
	})
}
