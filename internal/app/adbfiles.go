package app

// The files tab of an ADB tab: the device's folders in the file panel's
// tree and table (filebrowser.go), over its sync service.

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"finallyshell/internal/adb"
	"finallyshell/internal/remotefs"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// adbHome is the folder the files tab opens first: the device's own
// storage.
const adbHome = "/sdcard"

// adbFiles is the files tab's state for a session: the panel's folder,
// its table and its tree.
type adbFiles struct {
	browserState
}

// adbConn is the connection of s, if it is up.
func (a *App) adbConn(s *Session) *adb.Conn {
	m := a.adbMeter(s)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conn
}

// adbDir reads a folder of the device, as the file panel reads one of a
// server.
func adbDir(conn *adb.Conn) browserDir {
	return func(ctx context.Context, dir string) ([]remotefs.Entry, error) {
		entries, err := conn.ReadDir(ctx, dir)
		if err != nil {
			return nil, err
		}
		out := make([]remotefs.Entry, 0, len(entries))
		for _, e := range entries {
			out = append(out, adbEntry(e))
		}
		return out, nil
	}
}

// adbEntry is a file of the device as the file panel sees one.
func adbEntry(e adb.Entry) remotefs.Entry {
	return remotefs.Entry{
		Name: e.Name, Size: e.Size, Mode: e.Mode, ModTime: e.ModTime,
		Link: e.Link || e.Mode&fs.ModeSymlink != 0,
	}
}

// adbFileColumns are the table's: a device's sync service gives no owner,
// so the file panel's last column is not there.
func adbFileColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "name", Title: locales.ColName(), MinWidth: 160, Sortable: true},
		{ID: "size", Title: locales.ColSize(), Width: 90, Align: ui.End, Sortable: true},
		{ID: "type", Title: locales.ColType(), Width: 90, Sortable: true},
		{ID: "modified", Title: locales.ColModified(), Width: 140, Sortable: true},
		{ID: "perms", Title: locales.ColPermissions(), Width: 100, Sortable: true},
	}
}

// adbFilesTab is the page: the path and the buttons of the tab, the tree
// of the device's folders and the table of the one shown.
func (a *App) adbFilesTab(c *ui.Context) {
	s := a.connSession()
	if s == nil || !s.isADB() {
		return
	}
	f := &s.adbFiles
	conn := a.adbConn(s)
	if conn == nil {
		widgets.Muted(c, locales.AdbInstallNoDevice()).FontSize(12)
		return
	}
	if f.cwd == "" && !f.loading && f.err == nil {
		a.loadADBFiles(s, adbHome)
	}
	ui.Column(c).Fill().Children(func() {
		a.adbToolbar(c, s)
		if f.err != nil {
			ui.Text(c, fileErrorText(f.err)).TextColor(theme.Danger).Padding(4, 10).Wrap()
		}
		split := ui.Split(c, &a.treeWidth, func() { a.adbDirTree(c, s) },
			func() { a.adbFileTable(c, s) }).SplitReset(defaultTreeWidth).Grow(1)
		a.splitMoved(c, split, &a.treeWidth, treeLimits)
	})
}

// adbToolbar is the row above the panel: the folder, the ways through it,
// and what is done with what is chosen, as the file panel's own.
func (a *App) adbToolbar(c *ui.Context, s *Session) {
	f := &s.adbFiles
	ui.Row(c).Padding(4, 8).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
		if ui.TextInput(c, &f.pathEdit).Grow(1).MinWidth(0).Submitted() {
			a.loadADBFiles(s, adb.CleanDir(f.pathEdit))
		}
		if widgets.IconButton(c, "arrow-up", locales.ParentFolder()).Clicked() && f.cwd != "" {
			a.loadADBFiles(s, path.Dir(f.cwd))
		}
		if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Clicked() {
			a.loadADBFiles(s, f.cwd)
		}
		if widgets.IconButton(c, "house", locales.HomeFolder()).Clicked() {
			a.loadADBFiles(s, adbHome)
		}
		ui.Box(c).Size(1, 16).Background(colors.line).Margin(0, 4)
		if widgets.IconButton(c, "upload", locales.UploadFiles()).Clicked() {
			a.uploadADB(s, f.cwd, false)
		}
		if widgets.IconButton(c, "folder-up", locales.UploadFolder()).Clicked() {
			a.uploadADB(s, f.cwd, true)
		}
		e, one := f.entry(f.chosenNames())
		dl := widgets.IconButton(c, "download", locales.Download())
		if !one || e.IsDir() {
			dl.Opacity(0.4)
		}
		if dl.Clicked() && one && !e.IsDir() {
			a.downloadADB(s, f.cwd, e.Name)
		}
		if widgets.IconButton(c, "folder-plus", locales.NewFolder()).Clicked() {
			a.askNew(s, f.cwd, opMkdir)
		}
		del := widgets.IconButton(c, "trash-2", locales.Delete())
		if f.chosen.Len() == 0 {
			del.Opacity(0.4)
		}
		if del.Clicked() && f.chosen.Len() > 0 {
			a.askADBDeleteChosen(s)
		}
		if f.loading {
			ui.Spinner(c)
		}
	})
}

// adbDirTree is the tree of the device's folders, as the file panel's.
func (a *App) adbDirTree(c *ui.Context, s *Session) {
	f := &s.adbFiles
	a.folderTree(c, &f.browserState, func(dir string) { a.loadADBSubdirs(s, dir) },
		func(row ui.Element, dir string) {
			a.adbDropZone(row, s, dir)
			row.ContextMenu(func(m *ui.Menu) { a.adbTreeMenu(c, m, s, dir) })
		},
		func(dir string) { a.loadADBFiles(s, dir) })
}

// adbFileTable is the folder's entries, as the file panel's table is.
func (a *App) adbFileTable(c *ui.Context, s *Session) {
	f := &s.adbFiles
	zone := ui.Column(c).Fill()
	a.adbDropZone(zone, s, f.cwd)
	a.browserTable(c, &f.browserState, adbFileColumns(), zone, func(e remotefs.Entry) {
		// A double click or Enter opens a folder, or fetches a file.
		if e.IsDir() {
			a.loadADBFiles(s, path.Join(f.cwd, e.Name))
		} else {
			a.downloadADB(s, f.cwd, e.Name)
		}
	}, func(t ui.Element) {
		// F2 renames the row chosen; Delete, or Cmd+Backspace as in
		// Finder, deletes those chosen.
		if e, ok := f.entry(f.chosenNames()); ok && f.table.Shortcut(c, 0, ui.KeyF2) {
			a.askRename(s, path.Join(f.cwd, e.Name), e.IsDir())
		}
		del := f.table.Shortcut(c, 0, ui.KeyDelete)
		del = f.table.Shortcut(c, ui.Cmd, ui.KeyBackspace) || del
		if del && f.chosen.Len() > 0 {
			a.askADBDeleteChosen(s)
		}
		t.ContextMenu(func(m *ui.Menu) {
			names := f.chosenNames()
			e, one := f.entry(names)
			if one {
				if e.IsDir() {
					if m.Item(locales.OpenFolder()).Chosen() {
						a.loadADBFiles(s, path.Join(f.cwd, e.Name))
					}
				} else if m.Item(locales.Download()).Chosen() {
					a.downloadADB(s, f.cwd, e.Name)
				}
				m.Separator()
			}
			if m.Item(locales.UploadFiles()).Chosen() {
				a.uploadADB(s, f.cwd, false)
			}
			if m.Item(locales.UploadFolder()).Chosen() {
				a.uploadADB(s, f.cwd, true)
			}
			m.Separator()
			if m.Item(locales.NewFolder()).Chosen() {
				a.askNew(s, f.cwd, opMkdir)
			}
			if m.Item(locales.NewFile()).Chosen() {
				a.askNew(s, f.cwd, opCreate)
			}
			m.Separator()
			if m.Item(locales.Rename()).Shortcut(0, ui.KeyF2).Disabled(!one).Chosen() && one {
				a.askRename(s, path.Join(f.cwd, e.Name), e.IsDir())
			}
			if m.Item(locales.Delete()).Shortcut(0, ui.KeyDelete).Disabled(len(names) == 0).Chosen() && len(names) > 0 {
				a.askADBDeleteChosen(s)
			}
			m.Separator()
			if m.Item(locales.Refresh()).Chosen() {
				a.loadADBFiles(s, f.cwd)
			}
			if len(names) == 1 && m.Item(locales.CopyPath()).Chosen() {
				c.WriteClipboard(path.Join(f.cwd, names[0]))
			}
		})
	})
}

// adbTreeMenu is the context menu of the folder dir in the tree: it acts
// on dir, not on the current folder, and doesn't open it.
func (a *App) adbTreeMenu(c *ui.Context, m *ui.Menu, s *Session, dir string) {
	if m.Item(locales.OpenFolder()).Chosen() {
		a.loadADBFiles(s, dir)
	}
	m.Separator()
	if m.Item(locales.UploadFiles()).Chosen() {
		a.uploadADB(s, dir, false)
	}
	if m.Item(locales.UploadFolder()).Chosen() {
		a.uploadADB(s, dir, true)
	}
	if m.Item(locales.NewFolder()).Chosen() {
		a.askNew(s, dir, opMkdir)
	}
	m.Separator()
	if m.Item(locales.Refresh()).Chosen() {
		a.loadADBFiles(s, dir)
	}
	if m.Item(locales.CopyPath()).Chosen() {
		c.WriteClipboard(dir)
	}
}

// adbDropZone makes e take files dropped from the system's file manager, to
// push them into dir, and outlines it while they are over it.
func (a *App) adbDropZone(e ui.Element, s *Session, dir string) {
	if files := e.DroppedFiles(); len(files) > 0 && dir != "" {
		a.uploadADBFiles(s, dir, files)
	}
	if e.FileDragOver() && dir != "" {
		e.Border(2, theme.Accent).Background(theme.Accent.Alpha(0.06))
	}
}

// loadADBFiles lists dir in the tab's table, and makes it the folder
// shown.
func (a *App) loadADBFiles(s *Session, dir string) {
	conn := a.adbConn(s)
	if conn == nil {
		return
	}
	a.listFolder(&s.adbFiles.browserState, adbDir(conn), adbCtx(s), adb.CleanDir(dir), false,
		func(dir string) { a.loadADBSubdirs(s, dir) })
}

// loadADBSubdirs reads the directories in dir, for the tree.
func (a *App) loadADBSubdirs(s *Session, dir string) {
	conn := a.adbConn(s)
	if conn == nil {
		return
	}
	a.loadSubdirsInto(&s.adbFiles.browserState, adbDir(conn), adbCtx(s), adb.CleanDir(dir))
}

// adbCtx is the connection's context, or a background one before the
// session has one.
func adbCtx(s *Session) context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

// uploadADB asks for files of this computer, or a folder, and pushes them
// into dir of the device, each under its name.
func (a *App) uploadADB(s *Session, dir string, folder bool) {
	if dir == "" {
		return
	}
	title := locales.ChooseUploadFiles()
	if folder {
		title = locales.ChooseUploadFolder()
	}
	go func() {
		files, err := a.plat.OpenFiles(title, folder)
		if err != nil || len(files) == 0 {
			return
		}
		a.uploadADBFiles(s, dir, files)
	}()
}

// uploadADBFiles pushes local files, or folders with what they hold, into
// dir of the device, one after the other.
func (a *App) uploadADBFiles(s *Session, dir string, paths []string) {
	conn := a.adbConn(s)
	if conn == nil || dir == "" {
		return
	}
	ctx := adbCtx(s)
	go func() {
		var failure error
		for _, p := range paths {
			if err := a.pushADBPath(ctx, conn, p, dir); err != nil {
				failure = err
				break
			}
		}
		a.post(func() {
			if failure != nil {
				a.notify(failure.Error())
			} else {
				a.notify(locales.AdbUploaded())
				// What was just uploaded is chosen in the table, as the
				// file panel does for what it made.
				if dir == s.adbFiles.cwd {
					s.adbFiles.reveal = filepath.Base(paths[len(paths)-1])
				}
			}
			a.loadADBFiles(s, s.adbFiles.cwd)
		})
	}()
}

// pushADBPath copies a file or a folder of this computer into dir of the
// device, under its name.
func (a *App) pushADBPath(ctx context.Context, conn *adb.Conn, local, dir string) error {
	info, err := os.Stat(local)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return a.pushADBFile(ctx, conn, local, path.Join(dir, filepath.Base(local)))
	}
	root := path.Join(dir, filepath.Base(local))
	if err := conn.Mkdir(ctx, root); err != nil {
		return err
	}
	return filepath.WalkDir(local, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == local {
			return nil
		}
		rel, err := filepath.Rel(local, p)
		if err != nil {
			return err
		}
		target := path.Join(root, filepath.ToSlash(rel))
		if d.IsDir() {
			return conn.Mkdir(ctx, target)
		}
		return a.pushADBFile(ctx, conn, p, target)
	})
}

// pushADBFile copies one local file to the device.
func (a *App) pushADBFile(ctx context.Context, conn *adb.Conn, local, remote string) error {
	in, err := os.Open(local)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := conn.CreateFile(ctx, remote)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// downloadADB asks where to save a file of the device, and copies it
// there.
func (a *App) downloadADB(s *Session, dir, name string) {
	conn := a.adbConn(s)
	if conn == nil {
		return
	}
	go func() {
		to, err := a.plat.SaveFile(WindowMain, locales.AdbDownload(), name, "", nil)
		if err != nil || to == "" {
			return
		}
		err = a.pullADBFile(adbCtx(s), conn, path.Join(dir, name), to)
		a.post(func() {
			if err != nil {
				a.notify(err.Error())
			} else {
				a.notify(locales.AdbDownloaded(name))
			}
		})
	}()
}

// pullADBFile copies one file of the device to this computer.
func (a *App) pullADBFile(ctx context.Context, conn *adb.Conn, remote, local string) error {
	in, err := conn.OpenFile(ctx, remote)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(local)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// askADBDeleteChosen asks before deleting the rows chosen in the table.
func (a *App) askADBDeleteChosen(s *Session) {
	f := &s.adbFiles
	names := f.chosenNames()
	dirs := false
	for _, e := range f.entries {
		if f.chosen.Has(e.Name) && e.IsDir() {
			dirs = true
		}
	}
	a.askDelete(s, f.cwd, names, dirs)
}
