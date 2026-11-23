package app

import (
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The whole left side of the window, the server monitor over the session
// list, can be hidden, and the terminal takes the width. A button at the
// left end of the row of session tabs (sessiontabs.go) hides it, and shows
// it again; so does the row's context menu and the shortcut (scSidebar).
//
// The state is the settings' SidebarHidden, saved with the pane sizes.
// Full screen shows the terminal alone anyway, and leaves the setting as
// it was.

// setSidebarHidden hides the left side, or shows it, and saves the choice.
func (a *App) setSidebarHidden(hidden bool) {
	if a.sideHidden == hidden {
		return
	}
	a.sideHidden = hidden
	a.saveLayout()
}

// toggleSidebar shows the hidden left side, or hides it.
func (a *App) toggleSidebar() { a.setSidebarHidden(!a.sideHidden) }

// sidebarButton is the button at the left end of the row of session tabs
// that hides the left side, a chevron left, or shows it, a chevron right.
func (a *App) sidebarButton(c *ui.Context) {
	icon, label := "chevron-left", locales.HideSidebar()
	if a.sideHidden {
		icon, label = "chevron-right", locales.ShowSidebar()
	}
	if widgets.IconButton(c, icon, label).Clicked() {
		a.toggleSidebar()
	}
}

// sidebarMenu is the context menu of the row of session tabs: whether the
// left side shows, as the row's button does.
func (a *App) sidebarMenu(m *ui.Menu) {
	label := locales.HideSidebar()
	if a.sideHidden {
		label = locales.ShowSidebar()
	}
	if m.Item(label).Chosen() {
		a.toggleSidebar()
	}
}
