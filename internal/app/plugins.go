package app

import (
	"strconv"
	"strings"

	"finallyshell/internal/icons"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The plugin center: every feature that can be turned off, in one window, as
// QuickTerm's. The row of session tabs keeps one entry to it, before the
// settings, and the icons of the plugins the user pinned there: adding a
// feature then adds a line to the registry, not an icon of its own.
//
// A plugin is opened (a window of its own, or a page), which the row of tabs
// can pin, or a panel shown while it is on, which it cannot: a panel has no
// window to point an icon at. This registry drives the window, the row of
// tabs and the plugin's other entries alike, so that turning one off leaves
// no entry behind.
//
// The state is a list of overrides in settings.json, "<id>:0" and "<id>:p1",
// kept only where a plugin differs from its default: a new default then
// reaches the users who never touched it.

// The two kinds of plugin: one that is opened, and one that is a panel.
const (
	pluginOpen  = "open"
	pluginPanel = "panel"
)

// The plugins, as openPlugin knows them.
const (
	pluginAssets   = "assets"
	pluginManager  = "manager"
	pluginImport   = "import"
	pluginHostKeys = "hostkeys"
	pluginDanger   = "danger"
	pluginOps      = "ops"
	pluginPerf     = "perf"
	pluginStorage  = "storage"
	pluginRunCmd   = "runcmd"
)

// The categories, as the window's chips name them.
const (
	pluginCatAll      = "all" // not a category: every plugin
	pluginCatOverview = "overview"
	pluginCatOps      = "ops"
	pluginCatSecurity = "security"
	pluginCatUI       = "ui"
)

// pluginCategory is a chip of the window: its id, and its name.
type pluginCategory struct {
	id   string
	name func() string
}

// pluginCategories are the chips, in their order. A category without plugins
// shows nothing, so that a new one can land before its first plugin does.
var pluginCategories = []pluginCategory{
	{pluginCatOverview, locales.PluginsCatOverview},
	{pluginCatOps, locales.PluginsCatOps},
	{pluginCatSecurity, locales.PluginsCatSecurity},
	{pluginCatUI, locales.PluginsCatUi},
}

// pluginDef is a line of the registry: what the window shows, where the
// plugin sits, and whether it is on and on the row of tabs before the user
// says otherwise.
type pluginDef struct {
	id       string
	kind     string
	category string
	icon     string
	name     func() string
	desc     func() string
	// defaultOn and defaultPinned are defaults only: an override in the
	// settings says what the user made of them, and a plugin changed here
	// still applies to whoever never touched it.
	defaultOn     bool
	defaultPinned bool
}

// pluginList is the registry. An open plugin's entries are in openPlugin and
// those it hides; a panel plugin is drawn where pluginOn(id) holds.
var pluginList = []pluginDef{
	{
		id: pluginAssets, kind: pluginOpen, category: pluginCatOverview, icon: "asset",
		name: locales.PluginAssetsName, desc: locales.PluginAssetsDesc,
		defaultOn: true, defaultPinned: true,
	},
	{
		// Not pinned: the welcome page and the shortcut already open it, an
		// icon of it on the row would be a third entry.
		id: pluginManager, kind: pluginOpen, category: pluginCatOps, icon: "server",
		name: locales.PluginManagerName, desc: locales.PluginManagerDesc,
		defaultOn: true,
	},
	{
		id: pluginImport, kind: pluginOpen, category: pluginCatOps, icon: "import",
		name: locales.PluginImportName, desc: locales.PluginImportDesc,
		defaultOn: true,
	},
	{
		id: pluginHostKeys, kind: pluginOpen, category: pluginCatSecurity, icon: "key-round",
		name: locales.PluginHostKeysName, desc: locales.PluginHostKeysDesc,
		defaultOn: true,
	},
	{
		id: pluginDanger, kind: pluginOpen, category: pluginCatSecurity, icon: "triangle-alert",
		name: locales.PluginDangerName, desc: locales.PluginDangerDesc,
		defaultOn: true,
	},
	{
		id: pluginOps, kind: pluginOpen, category: pluginCatOps, icon: "file-text",
		name: locales.PluginOpsName, desc: locales.PluginOpsDesc,
		defaultOn: true,
	},
	{
		id: pluginPerf, kind: pluginOpen, category: pluginCatOps, icon: "cpu",
		name: locales.PluginPerfName, desc: locales.PluginPerfDesc,
		defaultOn: true,
	},
	{
		id: pluginStorage, kind: pluginOpen, category: pluginCatOps, icon: "trash-2",
		name: locales.PluginStorageName, desc: locales.PluginStorageDesc,
		defaultOn: true,
	},
	{
		id: pluginRunCmd, kind: pluginOpen, category: pluginCatOps, icon: "square-terminal",
		name: locales.PluginRunCmdName, desc: locales.PluginRunCmdDesc,
		defaultOn: true,
	},
}

func pluginByID(id string) *pluginDef {
	for i := range pluginList {
		if pluginList[i].id == id {
			return &pluginList[i]
		}
	}
	return nil
}

func pluginDefaultOn(id string) bool {
	p := pluginByID(id)
	return p != nil && p.defaultOn
}

func pluginDefaultPinned(id string) bool {
	p := pluginByID(id)
	return p != nil && p.defaultPinned
}

// pluginCanPin tells whether the plugin can be pinned to the row of tabs:
// only one that is opened has somewhere to go, a panel has not.
func pluginCanPin(id string) bool {
	p := pluginByID(id)
	return p != nil && p.kind != pluginPanel
}

// pluginCategoriesInUse are the categories with a plugin, in the
// registry's order.
func pluginCategoriesInUse() []pluginCategory {
	var out []pluginCategory
	for _, c := range pluginCategories {
		if pluginCategoryCount(c.id) > 0 {
			out = append(out, c)
		}
	}
	return out
}

// pluginsInCategory are the plugins of a category, all of them for
// pluginCatAll.
func pluginsInCategory(cat string) []pluginDef {
	if cat == pluginCatAll {
		return pluginList
	}
	var out []pluginDef
	for _, p := range pluginList {
		if p.category == cat {
			out = append(out, p)
		}
	}
	return out
}

func pluginCategoryCount(cat string) int { return len(pluginsInCategory(cat)) }

// The tags an override ends with, after "<id>:".
const (
	pluginOnTag     = "1"
	pluginOffTag    = "0"
	pluginPinOnTag  = "p1"
	pluginPinOffTag = "p0"
)

// pluginStore is the plugin states: the overrides the settings keep.
type pluginStore struct {
	overrides []string
}

// load reads the overrides the settings kept, nil for none.
func (s *pluginStore) load(list *[]string) {
	s.overrides = nil
	if list != nil {
		s.overrides = append(s.overrides, (*list)...)
	}
}

// list is the overrides to keep, nil where there are none, as the settings
// leave an unset list out.
func (s *pluginStore) list() *[]string {
	if len(s.overrides) == 0 {
		return nil
	}
	out := append([]string(nil), s.overrides...)
	return &out
}

func (s *pluginStore) isOn(id string) bool {
	if on, ok := s.tag(id, pluginOnTag, pluginOffTag); ok {
		return on
	}
	return pluginDefaultOn(id)
}

func (s *pluginStore) setOn(id string, on bool) {
	s.setTag(id, on, pluginOnTag, pluginOffTag, pluginDefaultOn(id))
}

func (s *pluginStore) isPinned(id string) bool {
	if on, ok := s.tag(id, pluginPinOnTag, pluginPinOffTag); ok {
		return on
	}
	return pluginDefaultPinned(id)
}

func (s *pluginStore) setPinned(id string, on bool) {
	s.setTag(id, on, pluginPinOnTag, pluginPinOffTag, pluginDefaultPinned(id))
}

// pinned are the plugins the row of tabs shows: pinned, still on, and opened
// by an icon. The three conditions are filtered here alone, so that turning
// a plugin off takes its icon with it.
func (s *pluginStore) pinned() []pluginDef {
	var out []pluginDef
	for _, p := range pluginList {
		if p.kind == pluginOpen && s.isOn(p.id) && s.isPinned(p.id) {
			out = append(out, p)
		}
	}
	return out
}

// tag is one family's value for a plugin: false, true, or ok false when the
// user never set it, which leaves the default.
func (s *pluginStore) tag(id, onTag, offTag string) (on, ok bool) {
	prefix := id + ":"
	for _, it := range s.overrides {
		if !strings.HasPrefix(it, prefix) {
			continue
		}
		switch strings.TrimPrefix(it, prefix) {
		case onTag:
			return true, true
		case offTag:
			return false, true
		}
	}
	return false, false
}

// setTag sets one family's value, dropping the override where it is the
// default: the other family's tag for the plugin stays.
func (s *pluginStore) setTag(id string, on bool, onTag, offTag string, def bool) {
	prefix := id + ":"
	kept := make([]string, 0, len(s.overrides)+1)
	for _, it := range s.overrides {
		if strings.HasPrefix(it, prefix) {
			rem := strings.TrimPrefix(it, prefix)
			if rem == onTag || rem == offTag {
				continue
			}
		}
		kept = append(kept, it)
	}
	if on != def {
		tag := offTag
		if on {
			tag = onTag
		}
		kept = append(kept, prefix+tag)
	}
	s.overrides = kept
}

// pluginOn tells whether the plugin is on, as its panel and entries ask.
func (a *App) pluginOn(id string) bool { return a.plugins.isOn(id) }

// savePlugins keeps the plugin states in the settings.
func (a *App) savePlugins() {
	a.settings.Plugins = a.plugins.list()
	a.saveSettings(a.settings)
}

// setPlugin turns a plugin on or off, and closes what it had open.
func (a *App) setPlugin(id string, on bool) {
	a.plugins.setOn(id, on)
	a.savePlugins()
	if !on {
		a.closePluginSurfaces(id)
	}
}

// pinPlugin pins a plugin to the row of tabs, or takes its icon away.
func (a *App) pinPlugin(id string, on bool) {
	a.plugins.setPinned(id, on)
	a.savePlugins()
}

// closePluginSurfaces closes what the plugin had open, so that a plugin
// turned off leaves nothing behind.
func (a *App) closePluginSurfaces(id string) {
	switch id {
	case pluginAssets:
		a.closeWindow(WindowAssets)
	case pluginManager:
		a.closeWindow(WindowManager)
	case pluginImport:
		a.closeWindow(WindowImport)
	case pluginHostKeys:
		a.closeWindow(WindowHostKeys)
	case pluginDanger:
		a.closeWindow(WindowDangerRules)
	case pluginOps:
		a.closeWindow(WindowOpsReport)
	case pluginPerf:
		a.closeWindow(WindowPerf)
	case pluginStorage:
		a.closeWindow(WindowStorage)
	case pluginRunCmd:
		a.closeWindow(WindowRunCmd)
	}
}

// openPlugin opens the plugin: the window, or the page, it stands for.
func (a *App) openPlugin(id string) {
	a.closeWindow(WindowPlugins) // the plugin takes its place
	switch id {
	case pluginAssets:
		a.openAssets()
	case pluginManager:
		a.openManager()
	case pluginImport:
		a.openImport()
	case pluginHostKeys:
		a.openHostKeys()
	case pluginDanger:
		a.openDangerRules()
	case pluginOps:
		a.openOpsReport()
	case pluginPerf:
		a.openPerf()
	case pluginStorage:
		a.openStorage()
	case pluginRunCmd:
		a.openRunCmd()
	}
}

// pluginsWindow is the plugin center's spec: it opens over the main window as
// the options do.
var pluginsWindow = WindowSpec{
	Kind:      WindowPlugins,
	Title:     locales.PluginsTitle,
	Width:     900,
	Height:    460,
	MinWidth:  720,
	MinHeight: 380,
}

// openPlugins opens the plugin center, or brings it to the front.
func (a *App) openPlugins() {
	if !a.windowOpen(WindowPlugins) {
		a.pluginCat = pluginCatAll
	}
	a.openWindow(pluginsWindow, a.pluginsView)
}

// pluginsView is the window: the note, the categories and the tiles.
func (a *App) pluginsView(c *ui.Context) {
	ui.Column(c).Fill().Padding(12, 14, 12, 14).Gap(10).Children(func() {
		ui.Text(c, locales.PluginsNote()).FontSize(11).TextColor(theme.TextMuted).MaxLines(2)
		a.pluginChips(c)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { a.pluginTiles(c) })
	})
}

// pluginChips are the categories: all of them, then each that has plugins.
func (a *App) pluginChips(c *ui.Context) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		a.pluginChip(c, pluginCatAll, locales.PluginsAll(), len(pluginList))
		for _, cat := range pluginCategoriesInUse() {
			a.pluginChip(c, cat.id, cat.name(), pluginCategoryCount(cat.id))
		}
		ui.Spacer(c)
	})
}

// pluginChip is one category, the chosen one as the primary button.
func (a *App) pluginChip(c *ui.Context, id, name string, n int) {
	label := name + " " + strconv.Itoa(n)
	var chip ui.Element
	if a.pluginCat == id {
		chip = ui.PrimaryButton(c, label)
	} else {
		chip = ui.Button(c, label)
	}
	if chip.Clicked() {
		a.pluginCat = id
	}
}

// pluginTile is one plugin: its icon, name and description over the status,
// the pin and the switch.
func (a *App) pluginTile(c *ui.Context, p pluginDef) {
	on := a.plugins.isOn(p.id)
	tile := ui.Column(c).Basis(0).Grow(1).Shrink(1).MinWidth(0).Height(108).
		Padding(10, 12, 8, 12).Radius(8).Border(1, theme.Border)
	if on {
		tile.Border(1, theme.Accent.Alpha(0.5))
	}
	tile.Children(func() {
		canOpen := on && p.kind == pluginOpen
		head := ui.Row(c).Gap(8).AlignItems(ui.Start).Grow(1).MinHeight(0)
		if canOpen {
			head.Cursor(ui.CursorPointer)
		}
		head.Children(func() {
			color := theme.TextMuted
			if on {
				color = theme.Accent
			}
			ui.Icon(c, icons.Get(p.icon)).FontSize(20).TextColor(color).Shrink(0)
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
				name := ui.Text(c, p.name()).FontSize(12).SingleLine().MinWidth(0)
				if on {
					name.FontWeight(600)
				}
				ui.Text(c, p.desc()).FontSize(10).TextColor(theme.TextMuted).MaxLines(2).Ellipsis("…")
			})
		})
		if canOpen && head.Clicked() {
			a.openPlugin(p.id)
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			// The switch's own state is read back below: the widget writes
			// to it as the user clicks it.
			status := locales.PluginsDisabled()
			if on {
				status = locales.PluginsEnabled()
				if p.kind == pluginOpen {
					status = locales.PluginsOpen()
				}
			}
			if ui.Button(c, status).FontSize(10).Disabled(!canOpen).Clicked() {
				a.openPlugin(p.id)
			}
			ui.Spacer(c)
			if on && pluginCanPin(p.id) {
				pinned := a.plugins.isPinned(p.id)
				label := locales.PluginsPin()
				if pinned {
					label = locales.PluginsPinned()
				}
				if ui.Button(c, label).FontSize(10).Clicked() {
					a.pinPlugin(p.id, !pinned)
				}
			}
			switched := on
			ui.Switch(c, &switched).Label(p.name()).Shrink(0)
			if switched != on {
				a.setPlugin(p.id, switched)
			}
		})
	})
}

// pluginTiles are the tiles of the category shown, pluginCols to a row, with
// empty ones after them so that the tiles keep their width.
func (a *App) pluginTiles(c *ui.Context) {
	list := pluginsInCategory(a.pluginCat)
	for i := 0; i < len(list); i += pluginCols {
		row := list[i:min(i+pluginCols, len(list))]
		ui.Row(c).Gap(8).Padding(0, 0, 8, 0).Children(func() {
			for _, p := range row {
				a.pluginTile(c, p)
			}
			for j := len(row); j < pluginCols; j++ {
				ui.Box(c).Basis(0).Grow(1).Shrink(1).MinWidth(0)
			}
		})
	}
}

// pluginCols is how many tiles a row of the plugin center has.
const pluginCols = 3
