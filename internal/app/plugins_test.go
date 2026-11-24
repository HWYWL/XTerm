package app

import (
	"strings"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The plugin center's state: the defaults, the overrides kept in the settings
// and read back, and what the row of tabs shows of them.
func TestPluginStore(t *testing.T) {
	var s pluginStore
	if !s.isOn(pluginAssets) || !s.isPinned(pluginAssets) {
		t.Errorf("defaults: on %v pinned %v", s.isOn(pluginAssets), s.isPinned(pluginAssets))
	}
	if s.isOn("nobody") || pluginCanPin("nobody") {
		t.Error("a plugin that is not in the registry")
	}

	s.setOn(pluginAssets, false)
	if s.isOn(pluginAssets) || strings.Join(s.overrides, ",") != "assets:0" {
		t.Errorf("off: %v", s.overrides)
	}
	// The assets alone are on the row to begin with, and off they leave it.
	if ids := s.pinned(); len(ids) != 0 {
		t.Errorf("pinned %v", ids)
	}
	// Back to the default, the tag goes: a new default reaches the user.
	s.setOn(pluginAssets, true)
	if len(s.overrides) != 0 || s.list() != nil {
		t.Errorf("back on: %v", s.overrides)
	}

	// The switch and the pin are two tags, and setting one keeps the other.
	s.setPinned(pluginManager, true)
	s.setOn(pluginManager, false)
	if strings.Join(s.overrides, ",") != "manager:p1,manager:0" {
		t.Errorf("overrides %v", s.overrides)
	}
	// Pinned but off, a plugin has no icon: the row shows what can be opened.
	if ids := s.pinned(); len(ids) != 1 || ids[0].id != pluginAssets {
		t.Errorf("pinned %v", ids)
	}

	// Kept in the settings and read back.
	var back pluginStore
	back.load(s.list())
	if !back.isPinned(pluginManager) || !back.isPinned(pluginAssets) || back.isOn(pluginManager) {
		t.Errorf("read back: %v", back.overrides)
	}
}

// The icon before the settings opens the plugin center: all the plugins, the
// chips that filter them, and a plugin off taking its icons with it.
func TestPluginCenter(t *testing.T) {
	a, p := testApp(t)
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()

	// The assets window open first, to see it close with the plugin.
	if err := tt.Click(locales.PluginAssetsName()); err != nil {
		t.Fatalf("no assets icon: %v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !a.windowOpen(WindowAssets) {
		t.Fatalf("no assets window; windows %v", p.windows)
	}

	if err := tt.Click(locales.PluginsTitle()); err != nil {
		t.Fatalf("no plugin center icon: %v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !a.windowOpen(WindowPlugins) {
		t.Fatalf("no plugin center; windows %v", p.windows)
	}

	w := p.show(t, WindowPlugins)
	w.Frame()
	for _, s := range []string{
		locales.PluginsNote(), locales.PluginsAll() + " 8",
		locales.PluginsCatOverview() + " 1", locales.PluginsCatOps() + " 5",
		locales.PluginsCatSecurity() + " 2",
		locales.PluginAssetsName(), locales.PluginManagerName(), locales.PluginImportName(),
		locales.PluginHostKeysName(), locales.PluginDangerName(),
		locales.PluginOpsName(), locales.PluginPerfName(), locales.PluginStorageName(),
	} {
		if !w.HasText(s) {
			t.Errorf("no %q; texts %q", s, w.Texts())
		}
	}

	// A chip of a category shows its plugins alone.
	if err := w.Click(locales.PluginsCatOverview() + " 1"); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	if _, ok := w.Find(locales.PluginManagerName()); ok {
		t.Errorf("overview shows the manager; texts %q", w.Texts())
	}
	if _, ok := w.Find(locales.PluginAssetsName()); !ok {
		t.Errorf("overview without the assets; texts %q", w.Texts())
	}
	if err := w.Click(locales.PluginsAll() + " 8"); err != nil {
		t.Fatal(err)
	}
	w.Frame()

	// A plugin switched off goes: its window, its icon, its tag kept.
	clickRole(t, w, ui.RoleSwitch, locales.PluginAssetsName())
	if a.plugins.isOn(pluginAssets) || a.windowOpen(WindowAssets) {
		t.Errorf("assets still there: on %v, windows %v", a.plugins.isOn(pluginAssets), p.windows)
	}
	if kept := config.LoadSettings().Plugins; kept == nil || !strings.Contains(strings.Join(*kept, ","), "assets:0") {
		t.Errorf("kept %v", kept)
	}
	tt.Frame()
	if _, ok := tt.Find(locales.PluginAssetsName()); ok {
		t.Errorf("the assets icon stayed; texts %q", tt.Texts())
	}

	// A plugin pinned is on the row of tabs, and unpinned off it: the
	// manager is the first of the two left with a pin.
	clickRole(t, w, ui.RoleButton, locales.PluginsPin())
	if !a.plugins.isPinned(pluginManager) || len(a.plugins.pinned()) != 1 {
		t.Errorf("pinned: %v", a.plugins.pinned())
	}
	clickRole(t, w, ui.RoleButton, locales.PluginsPinned())
	if a.plugins.isPinned(pluginManager) || len(a.plugins.pinned()) != 0 {
		t.Errorf("unpinned: %v", a.plugins.pinned())
	}

	// Open opens the plugin, the center giving way to it.
	if err := w.Click(locales.PluginsOpen()); err != nil {
		t.Fatal(err)
	}
	if !a.windowOpen(WindowManager) || a.windowOpen(WindowPlugins) {
		t.Errorf("manager %v, center %v", a.windowOpen(WindowManager), a.windowOpen(WindowPlugins))
	}

	// Escape closes the center, as the title bar's close button does.
	a.openPlugins()
	p.show(t, WindowPlugins)
	w.Key(0, ui.KeyEscape)
	if a.windowOpen(WindowPlugins) {
		t.Error("Escape left the plugin center open")
	}
}

// A plugin turned off takes the entries outside the center with it.
func TestPluginEntriesGone(t *testing.T) {
	a, p := testApp(t)
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	if err := tt.Click(locales.ConnectionManager()); err != nil {
		t.Fatalf("no manager button on the welcome page: %v; texts %q", err, tt.Texts())
	}
	a.closeWindow(WindowManager)

	a.setPlugin(pluginManager, false)
	tt.Frame()
	if _, ok := tt.Find(locales.ConnectionManager()); ok {
		t.Errorf("the welcome page kept the manager; texts %q", tt.Texts())
	}
	// The manager is back with the plugin.
	a.setPlugin(pluginManager, true)
	tt.Frame()
	if err := tt.Click(locales.ConnectionManager()); err != nil {
		t.Errorf("the manager button did not come back: %v", err)
	}
}
