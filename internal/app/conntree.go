package app

import (
	"slices"
	"strconv"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The left column has three pages, switched by the row of tabs at its top
// (leftTabs): the server monitor (sidebar.go), the session tabs
// (sidebar.go), and Connections, the saved hosts in their folders, as
// MobaXterm's session tree and Xshell's Sessions pane keep them in the
// main window. Connections is the connection manager's tree (manager.go),
// with its search, menus and dragging, in one column: the names, a dot on
// the hosts with a session open, and the rest in a tooltip. Opening a host
// there opens a session as the start page does, and the page stays, to
// open the next.
//
// Both trees read a.hosts, so a change in one shows in the other at once.
// Each has its own managerState: what is open, chosen and searched, and
// its questions, which this page asks in the main window. It never shows
// the deleted hosts, so it asks before deleting one, and has neither the
// manager's filter nor its sort.
//
// The page shown and the folders open are kept in the settings
// (leftPanelPage, connectionTreeExpanded), on this device only: the
// folders by their IDs, which stay as they are renamed and moved.

// The pages of the left panel, in its tabs' order. The sessions page is the
// one it opens on, and the system page the one a.hosts' monitor fills: the
// tabs leave the monitor out while it is in the right column.
const (
	leftSystem = iota
	leftSessions
	leftConnections
)

// leftPanel is the page the left panel shows, and what its Connections
// page keeps besides its tree.
type leftPanel struct {
	page int
	// focusSearch gives the page's search the keyboard in the next frame
	// it shows.
	focusSearch bool
	// saved are the IDs of the folders open, as the settings have them.
	saved []string
}

// connIndent is how far each level of the tree is indented, in DIPs.
const connIndent = 14

// restoreLeftPanel shows the page and opens the folders the settings
// keep: every folder before the user opened or closed any.
func (a *App) restoreLeftPanel() {
	switch a.settings.LeftPanelPage {
	case config.LeftPanelConnections:
		a.left.page = leftConnections
	case config.LeftPanelSystem:
		a.left.page = leftSystem
	}
	open := &a.connTree.tree.Open
	if ids := a.settings.ConnectionTreeExpanded; ids != nil {
		for _, id := range *ids {
			open.Add(managerItem{folder: id})
		}
	} else {
		for _, f := range a.hosts.Folders {
			open.Add(managerItem{folder: f.ID})
		}
	}
	a.left.saved = a.openFolders()
}

// openFolders are the IDs of the folders open in the page's tree, each
// after the one it is in.
func (a *App) openFolders() []string {
	out := []string{}
	for _, f := range a.allFolders() {
		if a.connTree.tree.Open.Has(managerItem{folder: f.ID}) {
			out = append(out, f.ID)
		}
	}
	return out
}

// keepFoldersOpen saves the folders open in the page's tree as they
// change: not while searching, as the search opens those holding what it
// finds, and closes them again once it is cleared.
func (a *App) keepFoldersOpen() {
	if a.connTree.searching {
		return
	}
	ids := a.openFolders()
	if slices.Equal(ids, a.left.saved) {
		return
	}
	a.left.saved = ids
	a.settings.ConnectionTreeExpanded = &ids
	a.saveSettings(a.settings)
}

// setLeftPage shows page in the left panel, and keeps the choice.
func (a *App) setLeftPage(page int) {
	if page == leftSystem && a.monitorOnRight() {
		page = leftSessions // the monitor's page is the right column's
	}
	if page == a.left.page {
		return
	}
	a.left.page = page
	a.settings.LeftPanelPage = ""
	switch page {
	case leftConnections:
		a.settings.LeftPanelPage = config.LeftPanelConnections
	case leftSystem:
		a.settings.LeftPanelPage = config.LeftPanelSystem
	}
	if page != leftSessions {
		a.endRename(false) // its field is in a row no longer shown
	}
	a.saveSettings(a.settings)
}

// leftPage is the page the left column shows: the monitor's page is the
// right column's while the monitor is there, so the session list shows.
func (a *App) leftPage() int {
	if a.left.page == leftSystem && a.monitorOnRight() {
		return leftSessions
	}
	return a.left.page
}

// leftTabs are the pages the left column shows, in the tabs' order, and
// their labels: the server monitor is left out while it is in the right
// column.
func (a *App) leftTabs() (pages []int, labels []string) {
	pages = []int{leftSystem, leftSessions, leftConnections}
	labels = []string{locales.System(), locales.SessionsTitle(a.sessionCount()), locales.ConnectionsPage()}
	if a.monitorOnRight() {
		return pages[1:], labels[1:]
	}
	return pages, labels
}

// switchLeftPanel is the shortcut's: the Connections page with the
// keyboard in its search, or back to the sessions, with the keyboard in
// the terminal.
func (a *App) switchLeftPanel() {
	if a.left.page == leftConnections {
		a.setLeftPage(leftSessions)
		if s := a.current(); s != nil && s.term() != nil {
			s.bar.toTerm = true
		}
		return
	}
	if a.fullScreen.on {
		a.setFullScreen(false) // to show it
	}
	a.setLeftPage(leftConnections)
	a.left.focusSearch = true
}

// leftTabBar is the pane at the top of the left column: its pages, in the
// order leftTabs gives them, as plain links, the one shown in the text's
// color and the others muted. It is a pane of its own, as the row of
// session tabs is, with the gap of the panes below it.
func (a *App) leftTabBar(c *ui.Context) {
	pages, labels := a.leftTabs()
	page := a.leftPage()
	if !slices.Contains(pages, page) {
		page = pages[0]
	}
	top, bottom := glassTone()
	bar := ui.Row(c).Height(sessionTabBarHeight).PaddingX(6).Gap(theme.Space(1.5)).
		AlignItems(ui.Center).Radius(sessionTabRadius).Border(1, colors.frame).Clip().
		Gradient(top, bottom, 180)
	bar.Children(func() {
		// Each page in a pill of its own, with its corners round as its
		// height, so that the links show their bounds apart. The pills
		// share the row alike, the labels centered, so that they fill it
		// and shrink together when more pages come.
		for i, p := range pages {
			pill := ui.Row(c).Basis(0).Grow(1).Shrink(1).MinWidth(0).Height(22).
				Padding(0, theme.Space(1)).Radius(11).Border(1, theme.Border).
				AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
			switch {
			case p == page:
				pill.Background(theme.Background)
			case pill.Hovered():
				pill.Background(theme.SurfaceHover)
			}
			pill.Children(func() {
				t := ui.Text(c, labels[i]).FontSize(11).SingleLine().MinWidth(0)
				if p == page {
					t.TextColor(theme.Text).FontWeight(600)
				} else {
					t.TextColor(theme.TextMuted)
				}
			})
			if pill.Clicked() {
				a.setLeftPage(p)
			}
		}
	})
}

// showMonitorOfConnection shows the server monitor's page when a session
// connects from the connections, the page the left column opens on: the
// monitor is what one wants to see next.
func (a *App) showMonitorOfConnection() {
	if a.left.page == leftConnections {
		a.setLeftPage(leftSystem)
	}
}

// leftToolBar is the row under the tabs, with the page's own buttons: a
// local terminal and a new tab over the session list, a new connection over
// the tree. A page with none has no row.
func (a *App) leftToolBar(c *ui.Context) {
	page := a.leftPage()
	if page != leftSessions && page != leftConnections {
		return
	}
	ui.Row(c).Padding(0, theme.Space(2), theme.Space(0.5), theme.Space(2)).
		Gap(2).AlignItems(ui.Center).Children(func() {
		ui.Spacer(c)
		a.leftPageButtons(c, page)
	})
}

// leftPageButtons are the buttons of page, in its row under the tabs: the
// local terminal and a new tab, or a new connection.
func (a *App) leftPageButtons(c *ui.Context, page int) {
	switch page {
	case leftConnections:
		if headerButton(c, "plus", locales.NewConnection()).Clicked() {
			m := &a.connTree
			a.newConnectionIn(m, a.treeTarget(m))
		}
	case leftSessions:
		if headerButton(c, "square-terminal", locales.LocalTerminal()).Clicked() {
			a.openLocal(nil)
		}
		if headerButton(c, "plus", locales.NewTab()).Clicked() {
			a.NewTab()
		}
	}
}

// headerButton is a button of the left column's tool row, and of the row of
// tabs: an icon button a little narrower than the others, so that the row
// stays compact in a sidebar of the default width.
func headerButton(c *ui.Context, icon, label string) ui.Element {
	return widgets.IconButton(c, icon, label).Width(20)
}

// connectionPage is the Connections page: the search over the tree.
func (a *App) connectionPage(c *ui.Context) {
	m := &a.connTree
	var later []func() // what changes the tree, once it is built
	ui.Column(c).Grow(1).MinHeight(0).Gap(4).Padding(0, 4, 4, 4).Children(func() {
		search := ui.SearchField(c, &m.query).Placeholder(locales.SearchConnections()).Label(locales.SearchConnections()).
			FillWidth().Margin(0, 2)
		if a.left.focusSearch {
			search.Focus()
			a.left.focusSearch = false
		}
		if search.Changed() {
			a.searchChanged(m)
		}
		if search.Shortcut(0, ui.KeyDown) {
			m.focus = true
		}
		if search.Submitted() {
			a.connectFound(m)
		}
		a.connTreeView(c, m, &later)
		if m.saveErr != nil {
			ui.Text(c, m.saveErr()).TextColor(theme.Danger).FontSize(12).Padding(0, 6)
		}
	})
	for _, fn := range later {
		fn()
	}
	a.keepFoldersOpen()
}

// connTreeView is the page's tree: one column, as the rows of the
// session list are, with its own arrows (the Prefix of its rows), so
// that the whole row takes the menu and the drops.
func (a *App) connTreeView(c *ui.Context, m *managerState, later *[]func()) {
	t := a.newManagerTree(m)
	choose := a.treeFrame(c, m, t)
	over := managerItem{}
	drop := a.treeDropZone(m, later, &over)
	var prefix ui.OutlineRow
	m.tree.Prefix = func(r ui.OutlineRow) { prefix = r }
	tree := ui.Outline(c, &m.tree, t.roots(), t.children, func(it managerItem) {
		a.connTreeRow(c, m, it, prefix, drop, later)
	}).Grow(1).MinHeight(0)
	drop(tree, managerItem{}, "")
	m.over = over
	tree.Children(func() {
		if t.search && len(t.roots()) == 0 {
			widgets.Muted(c, locales.NoMatchingHosts()).Padding(8, 10)
		} else if len(a.hosts.Live()) == 0 && len(a.hosts.Folders) == 0 {
			widgets.Muted(c, locales.ConnTreeEmpty()).Padding(8, 10)
		}
	})
	a.newItemsMenu(tree, m, "", later)
	a.finishTree(c, m, tree, choose, later)
}

// connTreeRow is the row of it, a folder or a host, as deep and open as
// r says.
func (a *App) connTreeRow(c *ui.Context, m *managerState, it managerItem, r ui.OutlineRow, drop func(ui.Element, managerItem, string), later *[]func()) {
	var f *config.Folder
	var h *config.Host
	into := ""
	if it.isHost() {
		if h = a.hosts.Host(it.host); h == nil {
			return
		}
		into = a.folderOf(h)
	} else {
		if f = a.hosts.Folder(it.folder); f == nil {
			return
		}
		into = f.ID
	}
	color, muted := theme.Text, theme.TextMuted
	if m.isChosen(it) {
		color, muted = theme.AccentText, theme.AccentText.Alpha(0.75)
	}
	row := ui.Row(c).Grow(1).MinWidth(0).AlignSelf(ui.Stretch).Gap(5).AlignItems(ui.Center).Padding(4, 8, 4, 4).Radius(4)
	if it == m.over {
		row.Background(theme.Accent.Alpha(0.14))
	}
	drop(row, it, into)
	// A list choosing one row does not choose it on a right-click: the
	// menu acts on the row it was opened on, chosen for it.
	row.ContextMenu(func(menu *ui.Menu) {
		*later = append(*later, func() {
			if i := m.rowOfItem(it); i >= 0 {
				m.row = i
			}
		})
		a.managerItemMenu(c, m, menu, it, later)
	})
	if h != nil {
		row.Tooltip(hostTooltip(h))
	}
	row.Children(func() {
		if r.Depth > 0 {
			ui.Box(c).Width(float32(r.Depth) * connIndent).Shrink(0)
		}
		arrow := r.Arrow().Size(16, 16).Center()
		if r.Branch {
			icon := "chevron-right"
			if r.Open {
				icon = "chevron-down"
			}
			arrow.Children(func() {
				ui.Icon(c, icons.Get(icon)).FontSize(13).TextColor(muted)
			})
		}
		if f != nil {
			ui.Icon(c, icons.Get("folder")).FontSize(15).TextColor(folderColor)
			ui.Text(c, f.Name).SingleLine().MinWidth(0).TextColor(color)
			return
		}
		ui.Icon(c, icons.Get("server")).FontSize(14).TextColor(color.Alpha(0.75))
		ui.Text(c, h.Title()).SingleLine().MinWidth(0).TextColor(color)
		if dot, ok := a.sessionDot(h); ok {
			widgets.StatusDot(c, dot)
		}
	})
}

// hostTooltip is the tooltip of a host's row: where it connects, and the
// first line of its notes.
func hostTooltip(h *config.Host) string {
	s := h.User + "@" + h.Addr + ":" + strconv.Itoa(h.PortOr22())
	if notes, _, _ := strings.Cut(h.Notes, "\n"); strings.TrimSpace(notes) != "" {
		s += "\n" + notes
	}
	return s
}

// sessionDot is the color of the dot of h's row: the color of its
// sessions' dots in the session list, of one connected, else of one
// connecting. ok is false when none is.
func (a *App) sessionDot(h *config.Host) (color ui.Color, ok bool) {
	for _, s := range a.sessions {
		if s.Host == nil || s.Host.ID != h.ID { // a new tab, a local terminal
			continue
		}
		switch s.status {
		case statusConnected:
			return s.statusColor(theme), true
		case statusConnecting:
			color, ok = s.statusColor(theme), true
		}
	}
	return color, ok
}
