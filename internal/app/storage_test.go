package app

import (
	"os"
	"path/filepath"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// The storage window counts what the config directory takes, and cleaning
// keeps the files that matter: the settings, the hosts and the background in
// use.
func TestStorageWindow(t *testing.T) {
	a, p := testApp(t)
	dir := config.Dir()
	if err := os.MkdirAll(config.HistoryDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.HistoryDir(), "host.txt"), make([]byte, 2048), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.AssetStatsFile), make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	inUse, err := config.SaveBackground([]byte("in use"), "a.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.SaveBackground([]byte("old"), "b.png"); err != nil {
		t.Fatal(err)
	}
	a.settings.TermBackground = inUse

	a.openPlugin(pluginStorage)
	w := p.show(t, WindowStorage)
	w.Frame()
	for _, s := range []string{
		locales.StorageNote(), locales.StorageHistory(), locales.StorageStats(),
		locales.StorageBackgrounds(), "2.0 KB", "1.0 KB", locales.StorageClean(),
	} {
		if !w.HasText(s) {
			t.Errorf("no %q; texts %q", s, w.Texts())
		}
	}
	if !w.HasText(locales.StorageHistoryNote("1")) {
		t.Errorf("the history count; texts %q", w.Texts())
	}

	// Cleaning the copies keeps the one the settings name.
	a.cleanBackgrounds()
	if copies := config.BackgroundCopies(); len(copies) != 1 || copies[0] != inUse {
		t.Errorf("left %v", copies)
	}
	a.loadStorage()
	w.Frame()
	if !w.HasText(locales.StorageBackgroundsNote("1")) {
		t.Errorf("the copies count; texts %q", w.Texts())
	}
}
