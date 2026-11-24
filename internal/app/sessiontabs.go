package app

import (
	"slices"

	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// sessionTabBarHeight is the height of the row of session tabs above the
// terminal, in DIPs.
const sessionTabBarHeight = 30

// sessionTabMaxWidth is how wide a session tab grows at most before its
// name is cut with an ellipsis, in DIPs.
const sessionTabMaxWidth = 180

// sessionTabs is the row of session tabs above the terminal, as
// FinalShell's: a tab for each session, the selected one raised, with a
// button that closes it, and, after the last tab, a + that opens a tab
// for the same connection as the tab before it. With no session at all
// there is no +: the panel's header has one.
//
// The row is the same list as the left panel's vertical one (sidebar.go):
// both select a.sessions[a.active], so choosing a session either way shows
// it, and a tab closed here goes through closeSession. The tabs scroll
// sideways where they do not fit, the + with them, as the bottom panel's
// do, and the wheel over them scrolls them (sessionTabsWheel).
func (a *App) sessionTabs(c *ui.Context) {
	top, bottom := glassTone()
	bar := ui.Row(c).Height(sessionTabBarHeight).PaddingX(6).Gap(2).AlignItems(ui.Center).
		Radius(sessionTabRadius).Border(1, colors.frame).Clip().
		Gradient(top, bottom, 180).
		ContextMenu(a.sidebarMenu)
	// What the tabs change runs once the row is built: closing a tab in
	// the tab would leave the tabs after it to build with fewer sessions
	// than the row was given.
	var closing *Session
	var after func()
	bar.Children(func() {
		// The button that hides the left side, and shows it again.
		a.sidebarButton(c)
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Scrollbars(false).
			TrackScroll(&a.tabsScroll).HandleInput(a.sessionTabsWheel).Children(func() {
			ui.Row(c).Gap(4).Shrink(0).AlignItems(ui.Center).Children(func() {
				for i, s := range a.sessions {
					if a.sessionTab(c, i, s, &after) {
						closing = s
					}
				}
				if len(a.sessions) > 0 && widgets.IconButton(c, "plus", locales.NewTabBeside()).Clicked() {
					a.newTabBeside()
				}
			})
		})
		// The plugin center, then the plugins pinned to the row, then the
		// options: the plugins not on the row have no icon but its entry
		// (plugins.go).
		if headerButton(c, "puzzle", locales.PluginsTitle()).Clicked() {
			a.openPlugins()
		}
		for _, p := range a.plugins.pinned() {
			id := p.id
			if headerButton(c, p.icon, p.name()).Clicked() {
				a.openPlugin(id)
			}
		}
		if headerButton(c, "settings", locales.Options()).Clicked() {
			a.openOptions()
		}
	})
	if closing != nil {
		a.closeSession(closing)
	}
	if after != nil {
		after()
	}
}

// sessionTab is the tab of s, at index i of the sessions. It reports
// whether the user closed it, and sets after to what its context menu
// chose, which runs once the row is built.
func (a *App) sessionTab(c *ui.Context, i int, s *Session, after *func()) (closed bool) {
	active := i == a.active
	// Each tab in a pill of its own, its corners round as its height, so
	// that the tabs show their bounds apart.
	tab := ui.Row(c).Key(s.ID).Gap(6).Padding(0, 8).Height(24).Radius(12).
		AlignItems(ui.Center).Shrink(0).Role(ui.RoleTab).Label(listLabel(s)).
		AccessSelected(active).Cursor(ui.CursorPointer).Border(1, theme.Border)
	switch {
	case active:
		tab.Background(theme.Background)
	case tab.Hovered():
		tab.Background(theme.SurfaceHover)
	}
	if tab.Clicked() {
		a.active = i
	}
	a.tabMenu(tab, s, &closed, after)
	tab.Children(func() {
		tabMark(c, s, active)
		name := ui.Text(c, listLabel(s)).SingleLine().MaxWidth(sessionTabMaxWidth).Ellipsis("…")
		if active {
			name.Bold()
		}
		// As on the rows of the session list, the close button shows on
		// the selected tab and on the one hovered.
		closeBtn := widgets.IconButton(c, "x", locales.CloseSession()).Size(18, 18)
		if !active && !tab.Hovered() {
			closeBtn.Invisible()
		}
		if closeBtn.Clicked() {
			closed = true
		}
	})
	return closed
}

// tabMenu is the context menu of s's tab, as FinalShell's: duplicate the
// tab, connect it or every tab, disconnect it, close it, close the other
// tabs or all of them.
func (a *App) tabMenu(tab ui.Element, s *Session, closed *bool, after *func()) {
	tab.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.DuplicateTab()).Disabled(!canDuplicate(s)).Chosen() {
			a.duplicateSession(s)
		}
		m.Separator()
		if m.Item(locales.Connect()).Disabled(!canConnect(s)).Chosen() {
			a.reconnect(s)
		}
		if m.Item(locales.ConnectAll()).Disabled(!a.anyTabToConnect()).Chosen() {
			tabs := slices.Clone(a.sessions)
			*after = func() { a.connectTabs(tabs) }
		}
		m.Separator()
		if m.Item(locales.Disconnect()).Disabled(s.status != statusConnected && s.status != statusConnecting).Chosen() {
			a.disconnectSession(s)
		}
		m.Separator()
		if m.Item(locales.Close()).Danger().Chosen() {
			*closed = true
		}
		m.Separator()
		if m.Item(locales.CloseOtherTabs()).Disabled(s.isPanelTab() || len(a.sessions) < 2).Chosen() {
			*after = func() { a.closeOtherTabs(s) }
		}
		if m.Item(locales.CloseAllTabs()).Chosen() {
			*after = func() { a.closeAllTabs() }
		}
	})
}

// tabMark is the icon or status dot at the left of a session tab: the
// program's icon of a local terminal, the app's window for a new tab, the
// tab's own icon for a panel row, and the status dot of a session.
func tabMark(c *ui.Context, s *Session, active bool) {
	switch {
	case s.isPanelTab():
		icon := "app-window"
		if s.panelTab >= 0 && s.panelTab < len(panelTabIcons) {
			icon = panelTabIcons[s.panelTab]
		}
		ui.Icon(c, icons.Get(icon)).FontSize(13).Shrink(0).TextColor(theme.TextMuted)
	case s.isNewTab():
		ui.Box(c).Size(8, 8).Center().Shrink(0).Children(func() {
			ui.Icon(c, icons.Get("app-window")).FontSize(12).TextColor(theme.TextMuted)
		})
	default:
		sessionMark(c, s, active)
	}
}

// newTabBeside opens a tab for the connection of the tab before the +:
// the last one, or the last one before it that has a connection, as
// FinalShell's + does. Without one, it opens an empty new tab.
func (a *App) newTabBeside() {
	for i := len(a.sessions) - 1; i >= 0; i-- {
		if s := a.sessions[i]; canDuplicate(s) {
			a.duplicateSession(s)
			return
		}
	}
	a.NewTab()
}

// canDuplicate reports whether a tab of s can be opened beside it: a
// session with a connection, of its own or local. A new tab, a panel row
// and no session at all cannot.
func canDuplicate(s *Session) bool {
	return s != nil && !s.isNewTab() && !s.isPanelTab()
}

// duplicateSession opens a tab for the connection of s, as "Duplicate
// session" and "Duplicate tab" do.
func (a *App) duplicateSession(s *Session) {
	if s.isLocal() {
		a.openLocal(nil)
		return
	}
	a.openSessionIn(nil, s.Host)
}

// canConnect reports whether the tab of s has a connection to open: a
// session with a host or a local program that is not connected, or
// connecting, already.
func canConnect(s *Session) bool {
	return canDuplicate(s) && s.status != statusConnected && s.status != statusConnecting
}

// anyTabToConnect reports whether some tab can be connected.
func (a *App) anyTabToConnect() bool {
	return slices.ContainsFunc(a.sessions, canConnect)
}

// connectTabs reconnects the sessions of tabs that can be connected, one
// after another, waiting for a password prompt's answer before the next,
// as connectEach does for the connections a folder holds.
func (a *App) connectTabs(tabs []*Session) {
	defer a.redraw()
	for len(tabs) > 0 {
		s := tabs[0]
		tabs = tabs[1:]
		if !canConnect(s) {
			continue
		}
		if s.isLocal() {
			a.reconnect(s)
			continue
		}
		again := func() { a.connectTabs(append([]*Session{s}, tabs...)) }
		if a.readChainSecrets(s.Host, again) {
			return
		}
		if need := a.missingSecret(s.Host); need != nil {
			rest := tabs
			a.askSecret(need, again)
			a.pwCancel = func() { a.connectTabs(rest) }
			return
		}
		a.reconnect(s)
	}
}

// disconnectSession ends the connection of s, as "Disconnect" in the
// session list does, and leaves the tab to reconnect.
func (a *App) disconnectSession(s *Session) {
	a.transfers.cancelSession(s)
	s.disconnect()
	s.status, s.err = statusClosed, nil
}

// closeOtherTabs closes every session but s, whose panel rows go with
// theirs, and keeps s selected.
func (a *App) closeOtherTabs(s *Session) {
	for _, o := range slices.Clone(a.sessions) {
		if o != s && !o.isPanelTab() {
			a.closeSession(o)
		}
	}
	if i := slices.Index(a.sessions, s); i >= 0 {
		a.active = i
	}
}

// closeAllTabs closes every session, which leaves the window on its start
// page, with no tab.
func (a *App) closeAllTabs() {
	for _, s := range slices.Clone(a.sessions) {
		if !s.isPanelTab() {
			a.closeSession(s)
		}
	}
}

// sessionTabsWheel has the vertical wheel, or a vertical swipe of a
// touchpad, over the session tabs scroll them sideways while they do not
// fit: down toward the last tab, as tabsWheel does for the bottom panel's.
func (a *App) sessionTabsWheel(ev ui.InputEvent) bool {
	s := &a.tabsScroll
	if ev.Kind != ui.InputScroll || ev.DX != 0 || ev.DY == 0 || s.MaxX <= 0 || ev.Mods&ui.Ctrl != 0 {
		return false
	}
	s.X = min(max(s.X+ev.DY, 0), s.MaxX)
	return true
}
