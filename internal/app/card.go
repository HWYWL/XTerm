package app

import "github.com/egoist/mygo/ui"

// The glass panes keep cardGap of space from each other and from the edge
// of the window, and cardRadius rounds them, as QuickShell does.
const (
	cardGap    = 8
	cardRadius = 18
	// sessionTabRadius rounds the row of session tabs, which is too short
	// for the corners of a pane.
	sessionTabRadius = 10
)

// glassTone is the light a pane of glass catches, from its top to its
// bottom, in the night and in the day.
func glassTone() (top, bottom ui.Color) {
	if theme.Dark {
		return ui.RGBA(255, 255, 255, 0.075), ui.RGBA(255, 255, 255, 0.032)
	}
	return ui.RGBA(255, 255, 255, 0.74), ui.RGBA(255, 255, 255, 0.52)
}

// card puts what fn builds in a pane of glass over the sky the window
// shows: bg fills it where it is not transparent, else the light of glass.
//
// The pane keeps cardGap from the panes and the edge around it, which the
// splits of the window count in: panes inside it are that much smaller.
func card(c *ui.Context, bg ui.Color, fn func()) ui.Element {
	return cardInset(c, cardGap, bg, fn)
}

// cardInset is card with gap in place of cardGap: the narrow column at the
// right keeps none, as its panes are tight already.
func cardInset(c *ui.Context, gap float32, bg ui.Color, fn func()) ui.Element {
	return ui.Column(c).Fill().Padding(gap).Children(func() {
		// Clipped, so that what fills the pane keeps inside its round
		// corners, as a table's rows would square them off.
		pane := ui.Column(c).Fill().Radius(cardRadius).Border(1, colors.frame).Clip()
		if bg.A == 0 {
			top, bottom := glassTone()
			pane = pane.Gradient(top, bottom, 180)
		} else {
			pane = pane.Background(bg)
		}
		pane.Children(fn)
	})
}
