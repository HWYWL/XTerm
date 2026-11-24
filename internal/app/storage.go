package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The storage window: what the files of the config directory take, as
// QuickTerm's storage panel shows its database and caches, and the cleaning
// of the ones the app can make again: the command history, the days of the
// assets window, and the copies of background pictures, but never the one in
// use, the hosts or the settings.

// storageState is what the window counted when it opened or last cleaned.
type storageState struct {
	total, history, stats, backgrounds int64
	histFiles, copies                  int
}

// storageWindow is its spec.
var storageWindow = WindowSpec{
	Kind:      WindowStorage,
	Title:     locales.StorageTitle,
	Width:     760,
	Height:    480,
	MinWidth:  620,
	MinHeight: 380,
}

// openStorage opens the window, counting the files afresh.
func (a *App) openStorage() {
	if !a.windowOpen(WindowStorage) {
		a.loadStorage()
	}
	a.openWindow(storageWindow, a.storageView)
}

// loadStorage counts the directory and its parts.
func (a *App) loadStorage() {
	st := storageState{}
	st.total, _ = dirUsage(config.Dir())
	st.history, st.histFiles = dirUsage(config.HistoryDir())
	if fi, err := os.Stat(filepath.Join(config.Dir(), config.AssetStatsFile)); err == nil {
		st.stats = fi.Size()
	}
	for _, name := range config.BackgroundCopies() {
		if path, err := config.BackgroundPath(name); err == nil {
			if fi, err := os.Stat(path); err == nil {
				st.backgrounds += fi.Size()
			}
		}
		st.copies++
	}
	a.storage = st
}

// dirUsage is the bytes and the files under path, zero for one not there.
func dirUsage(path string) (int64, int) {
	var bytes int64
	var files int
	filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			bytes += fi.Size()
			files++
		}
		return nil
	})
	return bytes, files
}

// humanSize is bytes as a person reads them: 1.2 MB, 340 KB, 12 B.
func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// storageView is the window: the directory's total, then a row a kind.
func (a *App) storageView(c *ui.Context) {
	ui.Column(c).Fill().Padding(12, 14, 12, 14).Gap(10).Children(func() {
		ui.Text(c, locales.StorageNote()).FontSize(11).TextColor(theme.TextMuted).MaxLines(2)
		ui.Text(c, locales.StorageTotal(humanSize(a.storage.total))).FontSize(11).
			TextColor(theme.TextMuted)
		a.storageRow(c, locales.StorageHistory(),
			locales.StorageHistoryNote(strconv.Itoa(a.storage.histFiles)), a.storage.history,
			a.storage.histFiles > 0, func() { a.clearAllHistory() })
		a.storageRow(c, locales.StorageStats(),
			locales.StorageStatsNote(), a.storage.stats,
			a.storage.stats > 0, func() { a.resetAssetStats() })
		a.storageRow(c, locales.StorageBackgrounds(),
			locales.StorageBackgroundsNote(strconv.Itoa(a.storage.copies)), a.storage.backgrounds,
			a.storage.copies > 1, a.cleanBackgrounds)
	})
}

// storageRow is one kind of files: its name and count, its size, and the
// button that asks before cleaning it.
func (a *App) storageRow(c *ui.Context, name, note string, size int64, cleanable bool, clean func()) {
	row := ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(8, 10, 8, 10).
		Radius(6).Border(1, theme.Border)
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(12).SingleLine()
			ui.Text(c, note).FontSize(10).TextColor(theme.TextMuted).SingleLine()
		})
		ui.Text(c, humanSize(size)).FontSize(12).TextColor(theme.TextMuted)
		if cleanable {
			if ui.Button(c, locales.StorageClean()).Clicked() {
				a.askClean(name, clean)
			}
		} else {
			ui.Text(c, locales.StorageNone()).FontSize(10).TextColor(theme.TextMuted)
		}
	})
}

// askClean asks on the platform's dialog, and cleans if the answer is yes,
// counting the files again.
func (a *App) askClean(what string, clean func()) {
	q := Confirmation{
		Title:   locales.AppTitle(),
		Message: locales.StorageConfirm(what),
		OK:      locales.StorageClean(),
		Cancel:  locales.Cancel(),
	}
	go func() {
		if a.plat.Confirm(q) {
			a.post(func() {
				clean()
				a.loadStorage()
				a.redraw()
			})
		}
	}()
}

// cleanBackgrounds deletes the copies of background pictures but the one the
// settings still name.
func (a *App) cleanBackgrounds() {
	for _, name := range config.BackgroundCopies() {
		if name != a.settings.TermBackground {
			config.RemoveBackground(name)
		}
	}
}
