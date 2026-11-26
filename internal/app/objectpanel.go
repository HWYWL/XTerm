package app

import (
	"fmt"
	"path"
	"strings"

	"finallyshell/internal/objectstore"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// objectPrompt is a name the object panel asks for, as a new folder's.
type objectPrompt struct {
	open  bool
	title string
	value string
	onOK  func(string)
}

// objectDeleteAsk confirms the deletion of what the panel chose.
type objectDeleteAsk struct {
	open    bool
	s       *Session
	entries []objectstore.Entry
}

// objectKind names an entry's type as the file panel does: a folder, a
// file of an extension, or a file.
func objectKind(e objectstore.Entry) string {
	if e.Dir {
		return locales.KindFolder()
	}
	if ext := path.Ext(e.Name); len(ext) > 1 && len(ext) <= 8 && strings.Trim(ext[1:], "0123456789") != "" {
		return locales.KindExtFile(strings.ToUpper(ext[1:]))
	}
	return locales.KindFile()
}

// objectColumns are the table's: an object has no permissions and no
// owner, but its size, its kind and its time.
func objectColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "name", Title: locales.ColName(), MinWidth: 180},
		{ID: "size", Title: locales.ColSize(), Width: 90, Align: ui.End},
		{ID: "type", Title: locales.ColType(), Width: 90},
		{ID: "modified", Title: locales.ColModified(), Width: 140},
	}
}

// objectArea is the tab of an object storage session: the storage's files
// instead of a terminal, as the file panel shows a server's.
func (a *App) objectArea(c *ui.Context) {
	s := a.connSession()
	if s == nil || !s.isObject() {
		return
	}
	if s.status != statusConnected || s.object == nil {
		ui.Column(c).Fill().Center().Gap(8).Children(func() {
			if s.status == statusConnecting {
				ui.Spinner(c).Label(locales.ObjectLoading())
			}
			widgets.Muted(c, s.statusText())
			if s.err != nil {
				ui.Text(c, describeError(s.err)).TextColor(theme.Danger).Wrap().Width(520)
			}
		})
		return
	}
	f := &s.objects
	ui.Column(c).Fill().Background(theme.Background).Children(func() {
		a.objectHeader(c, s)
		a.objectToolbar(c, s)
		if f.err != nil {
			ui.Text(c, describeError(f.err)).TextColor(theme.Danger).Padding(4, 12).Wrap()
		}
		a.objectTable(c, s)
		a.objectTransferTray(c, s)
	})
}

// objectHeader is the row of the storage's name and where it is.
func (a *App) objectHeader(c *ui.Context, s *Session) {
	h := s.Host
	where := s.object.Kind()
	if h.Object != nil && h.Object.Bucket != "" {
		where += " · " + h.Object.Bucket
	}
	if h.Object != nil && h.Object.Provider == "local" {
		where = s.object.Kind() + " · " + h.Object.LocalPath
	}
	ui.Row(c).Padding(8, 12).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
		a.hostIcon(c, s.Host, 14, theme.Accent)
		ui.Text(c, h.Name).FontSize(13).Bold().SingleLine()
		widgets.Muted(c, where).FontSize(11.5).SingleLine().Shrink(1).MinWidth(0)
		ui.Spacer(c)
		if f := &s.objects; f.busy != "" {
			ui.Spinner(c)
			widgets.Muted(c, f.busy).FontSize(11.5).SingleLine()
		}
	})
}

// objectToolbar is the row above the table: the folder, the ways through
// it, and what is done with what is chosen.
func (a *App) objectToolbar(c *ui.Context, s *Session) {
	f := &s.objects
	ui.Row(c).Padding(4, 8).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
		if ui.TextInput(c, &f.pathEdit).Grow(1).Submitted() {
			a.objectOpen(s, f.pathEdit)
		}
		if widgets.IconButton(c, "arrow-up", locales.ParentFolder()).Clicked() && f.cwd != "" {
			a.objectOpen(s, objectUp(f.cwd))
		}
		if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Clicked() {
			a.objectOpen(s, f.cwd)
		}
		if widgets.IconButton(c, "house", locales.HomeFolder()).Clicked() && s.object != nil {
			a.objectOpen(s, s.object.Home())
		}
		ui.Box(c).Size(1, 16).Background(colors.line).Margin(0, 4)
		if widgets.IconButton(c, "upload", locales.UploadFiles()).Clicked() {
			a.objectUpload(s, f.cwd)
		}
		names := f.chosenKeys()
		dl := widgets.IconButton(c, "download", locales.Download())
		if len(names) != 1 {
			dl.Opacity(0.4)
		}
		if dl.Clicked() && len(names) == 1 {
			if e, ok := f.entry(names); ok && !e.Dir {
				a.objectDownload(s, e)
			}
		}
		if widgets.IconButton(c, "folder-plus", locales.NewFolder()).Clicked() {
			a.askObjectFolder(s, f.cwd)
		}
		del := widgets.IconButton(c, "trash-2", locales.Delete())
		if len(names) == 0 {
			del.Opacity(0.4)
		}
		if del.Clicked() && len(names) > 0 {
			a.askObjectDelete(s)
		}
		if f.loading {
			ui.Spinner(c)
		}
	})
}

// objectTable is the folder's entries, as the file panel's table is.
func (a *App) objectTable(c *ui.Context, s *Session) {
	f := &s.objects
	entries := f.entries
	f.table.Selection = &f.chosen
	f.table.Selected = &f.row
	f.table.Key = func(i int) any { return entries[i].Key }
	f.table.Label = func(i int) string { return entries[i].Name }
	zone := ui.Column(c).Fill()
	if files := zone.DroppedFiles(); len(files) > 0 {
		a.objectUploadPaths(s, f.cwd, files)
	}
	if zone.FileDragOver() {
		zone.Border(2, theme.Accent).Background(theme.Accent.Alpha(0.06))
	}
	zone.Children(func() {
		if len(entries) == 0 && !f.loading {
			widgets.Muted(c, locales.ObjectEmpty()).Padding(12, 14)
		}
		t := ui.Table(c, &f.table, objectColumns(), len(entries), func(row, col int) {
			e := entries[row]
			switch col {
			case 0:
				// The icons are the file panel's: the folder's, and the one
				// the icon theme gives the name's type.
				ui.Row(c).Gap(6).Children(func() {
					a.fileIcon(c, e.Name, e.Dir, false, false)
					name := e.Name
					if e.Dir {
						name += "/"
					}
					ui.Text(c, name).SingleLine()
				})
			case 1:
				if !e.Dir {
					ui.Text(c, formatSize(e.Size))
				}
			case 2:
				ui.Text(c, objectKind(e)).SingleLine()
			case 3:
				if !e.ModTime.IsZero() {
					ui.Text(c, e.ModTime.Local().Format("2006/01/02 15:04")).SingleLine()
				}
			}
		}).Grow(1)
		if t.Submitted() && f.row >= 0 && f.row < len(entries) {
			e := entries[f.row]
			if e.Dir {
				a.objectOpen(s, e.Key)
			} else {
				a.objectDownload(s, e)
			}
		}
		del := f.table.Shortcut(c, 0, ui.KeyDelete)
		del = f.table.Shortcut(c, ui.Cmd, ui.KeyBackspace) || del
		if del && f.chosen.Len() > 0 {
			a.askObjectDelete(s)
		}
		t.ContextMenu(func(m *ui.Menu) {
			names := f.chosenKeys()
			if e, ok := f.entry(names); ok {
				if e.Dir {
					if m.Item(locales.OpenFolder()).Chosen() {
						a.objectOpen(s, e.Key)
					}
				} else if m.Item(locales.Download()).Chosen() {
					a.objectDownload(s, e)
				}
				m.Separator()
			}
			if m.Item(locales.NewFolder()).Chosen() {
				a.askObjectFolder(s, f.cwd)
			}
			if m.Item(locales.UploadFiles()).Chosen() {
				a.objectUpload(s, f.cwd)
			}
			m.Separator()
			if m.Item(locales.Refresh()).Chosen() {
				a.objectOpen(s, f.cwd)
			}
			if len(names) == 1 && m.Item(locales.CopyPath()).Chosen() {
				c.WriteClipboard("/" + names[0])
			}
		})
	})
}

// askObjectDelete asks whether to delete what the panel chose.
func (a *App) askObjectDelete(s *Session) {
	f := &s.objects
	var entries []objectstore.Entry
	for _, e := range f.entries {
		if f.chosen.Has(e.Key) {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return
	}
	a.objectDeleteAsk = &objectDeleteAsk{open: true, s: s, entries: entries}
}

// askObjectFolder asks for the name of a folder to make in dir.
func (a *App) askObjectFolder(s *Session, dir string) {
	a.objectPrompt = &objectPrompt{
		open: true, title: locales.ObjectFolderName(), value: "",
		onOK: func(name string) {
			if name = strings.TrimSpace(name); name != "" {
				a.objectMkdir(s, dir, name)
			}
		},
	}
}

// objectDialogs draws the panel's dialogs, and does what they were asked.
func (a *App) objectDialogs(c *ui.Context) {
	if p := a.objectPrompt; p != nil {
		open := p.open
		ui.Modal(c, &open, func() {
			ui.Column(c).Width(360).Gap(12).Children(func() {
				ui.Text(c, p.title).FontSize(15).Bold()
				if ui.TextInput(c, &p.value).Submitted() {
					open = false
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, locales.Cancel()).Clicked() {
						open = false
					}
					if ui.Button(c, locales.Ok()).Clicked() {
						open = false
					}
				})
			})
		})
		if !open {
			if p.open && p.onOK != nil {
				p.onOK(p.value)
			}
			a.objectPrompt = nil
		} else {
			p.open = open
		}
	}
	if d := a.objectDeleteAsk; d != nil {
		open := d.open
		ui.Modal(c, &open, func() {
			ui.Column(c).Width(400).Gap(12).Children(func() {
				ui.Text(c, locales.Delete()).FontSize(15).Bold()
				ui.Text(c, fmt.Sprintf(locales.ObjectDeleteAsk(), len(d.entries))).Wrap()
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, locales.Cancel()).Clicked() {
						open = false
					}
					if ui.Button(c, locales.Delete()).Clicked() {
						open = false
						a.objectDelete(d.s, d.entries)
					}
				})
			})
		})
		if !open {
			a.objectDeleteAsk = nil
		} else {
			d.open = open
		}
	}
}
