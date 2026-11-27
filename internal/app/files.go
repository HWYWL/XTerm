package app

import (
	"cmp"
	"context"
	"path"
	"strings"

	"finallyshell/internal/remotefs"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
)

// fileState is the file panel of a session: what its folders hold, and
// the tree beside them (filebrowser.go).
type fileState struct {
	browserState
}

type sftpError struct{ err error }

func (e *sftpError) Error() string { return "sftp: " + e.err.Error() }

type dirError struct {
	dir string
	err error
}

func (e *dirError) Error() string { return e.dir + ": " + e.err.Error() }

func fileErrorText(err error) string {
	switch e := err.(type) {
	case *sftpError:
		return locales.SftpUnavailable(e.err.Error())
	case *dirError:
		return locales.CannotOpenFolder(e.dir, e.err.Error())
	}
	return err.Error()
}

func (f *fileState) init() { f.browserState.init() }

// sftpDir reads a folder of the server the client is on.
func sftpDir(sc *sftp.Client) browserDir {
	return func(ctx context.Context, dir string) ([]remotefs.Entry, error) {
		return remotefs.ReadDir(sc, dir)
	}
}

// openDir lists dir in the table and makes it the current directory.
func (a *App) openDir(s *Session, dir string) { a.listDir(s, dir, false) }

// listDir is openDir; quiet leaves the panel as it was, with no error,
// when dir can't be read, as when it follows the shell into a folder
// SFTP can't open.
func (a *App) listDir(s *Session, dir string, quiet bool) {
	sc := s.sftp
	if sc == nil {
		return
	}
	a.listFolder(&s.files.browserState, sftpDir(sc), s.ctx, dir, quiet,
		func(dir string) { a.loadSubdirs(s, dir) })
}

// refreshIfShowing lists dir again when the session shows it.
func (a *App) refreshIfShowing(s *Session, dir string) {
	if s.files.cwd == dir {
		a.openDir(s, dir)
	}
	delete(s.files.subdirs, dir)
}

// loadSubdirs reads the directories in dir, for the tree.
func (a *App) loadSubdirs(s *Session, dir string) {
	sc := s.sftp
	if sc == nil {
		return
	}
	a.loadSubdirsInto(&s.files.browserState, sftpDir(sc), s.ctx, dir)
}

func dirNames(dir string, entries []remotefs.Entry) []string {
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, path.Join(dir, e.Name))
		}
	}
	return names
}

// compareEntries orders two entries of the same kind, two files or two
// folders, by the column whose ID is by: by their names for "name" and
// for none, and by what the column shows otherwise.
func compareEntries(by string, x, y remotefs.Entry) int {
	switch by {
	case "size":
		return cmp.Compare(x.Size, y.Size)
	case "type":
		return strings.Compare(entryKind(x), entryKind(y))
	case "modified":
		return x.ModTime.Compare(y.ModTime)
	case "perms":
		return cmp.Compare(x.Mode.Perm(), y.Mode.Perm())
	case "owner":
		if c := cmp.Compare(x.UID, y.UID); c != 0 {
			return c
		}
		return cmp.Compare(x.GID, y.GID)
	}
	return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
}

func entryKind(e remotefs.Entry) string {
	switch {
	case e.IsDir():
		return locales.KindFolder()
	case e.Broken:
		return locales.KindBrokenLink()
	case e.Link:
		return locales.KindLink()
	}
	if ext := path.Ext(e.Name); len(ext) > 1 && len(ext) <= 8 && strings.Trim(ext[1:], "0123456789") != "" {
		return locales.KindExtFile(strings.ToUpper(ext[1:]))
	}
	return locales.KindFile()
}

func fileColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "name", Title: locales.ColName(), MinWidth: 160, Sortable: true},
		{ID: "size", Title: locales.ColSize(), Width: 90, Align: ui.End, Sortable: true},
		{ID: "type", Title: locales.ColType(), Width: 90, Sortable: true},
		{ID: "modified", Title: locales.ColModified(), Width: 140, Sortable: true},
		{ID: "perms", Title: locales.ColPermissions(), Width: 100, Sortable: true},
		{ID: "owner", Title: locales.ColOwner(), Width: 96, Sortable: true},
	}
}

// filePanel browses the active session's server over SFTP.
func (a *App) filePanel(c *ui.Context) {
	s := a.connSession()
	if s == nil || s.isNewTab() {
		return
	}
	if s.isLocal() {
		localUnavailable(c)
		return
	}
	f := &s.files
	ui.Column(c.Key(s.ID)).Grow(1).Children(func() {
		ui.Row(c).Padding(4, 8).Gap(4).BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			if ui.TextInput(c, &f.pathEdit).Grow(1).Submitted() {
				a.openDir(s, f.pathEdit)
			}
			if widgets.IconButton(c, "arrow-up", locales.ParentFolder()).Clicked() && f.cwd != "" {
				a.openDir(s, path.Dir(f.cwd))
			}
			if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Clicked() && f.cwd != "" {
				a.openDir(s, f.cwd)
			}
			if widgets.IconButton(c, "house", locales.HomeFolder()).Clicked() && s.sftp != nil {
				if home, err := s.sftp.Getwd(); err == nil {
					a.openDir(s, home)
				}
			}
			a.followButton(c, s)
			ui.Box(c).Size(1, 16).Background(colors.line).Margin(0, 4)
			// Their context menus also have the archive transfers.
			uploadMenu := func(m *ui.Menu) { a.uploadMenuItems(m, s, f.cwd) }
			if widgets.IconButton(c, "upload", locales.UploadFiles()).ContextMenu(uploadMenu).Clicked() {
				a.pickAndUpload(s, f.cwd, false)
			}
			if widgets.IconButton(c, "folder-up", locales.UploadFolder()).ContextMenu(uploadMenu).Clicked() {
				a.pickAndUpload(s, f.cwd, true)
			}
			names := f.chosenNames()
			dl := widgets.IconButton(c, "download", locales.Download())
			dl.ContextMenu(func(m *ui.Menu) { a.downloadMenuItems(m, s, f.chosenNames()) })
			if len(names) == 0 {
				dl.Opacity(0.4)
			}
			if dl.Clicked() && len(names) > 0 {
				a.download(s, names, "")
			}
			if f.loading {
				ui.Spinner(c)
			}
			ui.Box(c).Size(1, 16).Background(colors.line).Margin(0, 4)
			a.downloadsButton(c)
		})
		if f.err != nil {
			ui.Text(c, fileErrorText(f.err)).TextColor(theme.Danger).Padding(4, 10)
		}
		split := ui.Split(c, &a.treeWidth, func() { a.dirTree(c, s) }, func() { a.fileTable(c, s) }).SplitReset(defaultTreeWidth).Grow(1)
		a.splitMoved(c, split, &a.treeWidth, treeLimits)
	})
}

// dropZone makes e take files dropped from the system's file manager, to
// upload them into dir, and outlines it while they are over it.
func (a *App) dropZone(e ui.Element, s *Session, dir string) {
	if files := e.DroppedFiles(); len(files) > 0 && dir != "" {
		a.upload(s, files, dir)
	}
	if e.FileDragOver() && dir != "" {
		e.Border(2, theme.Accent).Background(theme.Accent.Alpha(0.06))
	}
}

func (a *App) dirTree(c *ui.Context, s *Session) {
	f := &s.files
	a.folderTree(c, &f.browserState, func(dir string) { a.loadSubdirs(s, dir) },
		func(row ui.Element, dir string) {
			a.dropZone(row, s, dir)
			row.ContextMenu(func(m *ui.Menu) { a.treeMenu(c, m, s, dir) })
		},
		func(dir string) { a.openDir(s, dir) })
}

func (a *App) fileTable(c *ui.Context, s *Session) {
	f := &s.files
	zone := ui.Column(c).Fill()
	a.dropZone(zone, s, f.cwd)
	a.browserTable(c, &f.browserState, fileColumns(), zone, func(e remotefs.Entry) {
		// A double click or Enter opens a folder, or a file in the editor.
		if e.IsDir() {
			a.openDir(s, path.Join(f.cwd, e.Name))
		} else if editable(e) {
			a.openRemoteFile(s, path.Join(f.cwd, e.Name))
		}
	}, func(t ui.Element) {
		// F2 renames the row chosen; Delete, or Cmd+Backspace as in
		// Finder, deletes those chosen.
		rename := f.table.Shortcut(c, 0, ui.KeyF2)
		del := f.table.Shortcut(c, 0, ui.KeyDelete)
		del = f.table.Shortcut(c, ui.Cmd, ui.KeyBackspace) || del
		if e, ok := f.entry(f.chosenNames()); ok && rename {
			a.askRename(s, path.Join(f.cwd, e.Name), e.IsDir())
		}
		if del && f.chosen.Len() > 0 {
			a.askDeleteChosen(s)
		}
		t.ContextMenu(func(m *ui.Menu) {
			names := f.chosenNames()
			if e, ok := f.entry(names); ok && editable(e) {
				if m.Item(locales.Edit()).Chosen() {
					a.openRemoteFile(s, path.Join(f.cwd, e.Name))
				}
				m.Separator()
			}
			a.downloadMenuItems(m, s, names)
			m.Separator()
			a.uploadMenuItems(m, s, f.cwd)
			m.Separator()
			if m.Item(locales.NewFolder()).Chosen() {
				a.askNew(s, f.cwd, opMkdir)
			}
			if m.Item(locales.NewFile()).Chosen() {
				a.askNew(s, f.cwd, opCreate)
			}
			m.Separator()
			e, one := f.entry(names)
			if m.Item(locales.Rename()).Shortcut(0, ui.KeyF2).Disabled(!one).Chosen() && one {
				a.askRename(s, path.Join(f.cwd, e.Name), e.IsDir())
			}
			if m.Item(locales.Delete()).Shortcut(0, ui.KeyDelete).Disabled(len(names) == 0).Chosen() && len(names) > 0 {
				a.askDeleteChosen(s)
			}
			m.Separator()
			if m.Item(locales.Refresh()).Chosen() {
				a.openDir(s, f.cwd)
			}
			if len(names) == 1 && m.Item(locales.CopyPath()).Chosen() {
				c.WriteClipboard(path.Join(f.cwd, names[0]))
			}
		})
	})
}
