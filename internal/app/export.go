package app

// Exporting the saved connections, as the menu of the room past the trees'
// rows does: the folders and the live hosts, written as JSON where the
// user says. It is the document hosts.json holds, so the file can be kept
// as a copy of the connections too.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// exportName is the name the export dialog starts with.
const exportName = "connections.json"

// exportStore is what an export holds: the folders and the live hosts, as
// hosts.json keeps them, without the deleted ones, whose tombstones mean
// nothing outside this app.
func (a *App) exportStore() *config.Store {
	s := config.NewStore()
	s.Folders = append([]*config.Folder{}, a.hosts.Folders...)
	s.Hosts = a.hosts.Live()
	return s
}

// exportConnections asks where to save the connections, over the window
// parent, and writes them there as JSON: with no password, which the
// credential store keeps, not hosts.json.
func (a *App) exportConnections(parent WindowKind) {
	data, err := json.MarshalIndent(a.exportStore(), "", "  ")
	if err != nil {
		a.notify(locales.ExportFailed(err.Error()))
		return
	}
	data = append(data, '\n')
	start := filepath.Join(a.plat.DownloadsDir(), exportName)
	go func() {
		path, err := a.plat.SaveFile(parent, locales.ExportConnectionsTitle(), start, locales.JsonFiles(), []string{"json"})
		if err == nil && path != "" {
			err = os.WriteFile(path, data, 0o644)
		}
		a.post(func() {
			switch {
			case err != nil:
				a.notify(locales.ExportFailed(err.Error()))
			case path != "":
				a.notify(locales.Exported(path))
			}
		})
	}()
}
