package app

// What the file panel keeps of a folder, and how it draws it, for every
// set of files the app browses: a server's over SFTP (files.go) and a
// device's over ADB (adbfiles.go).

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"finallyshell/internal/remotefs"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// browserState is the state of a file panel: the folder shown, its
// entries, what is chosen, the order of the table, and the tree of the
// folders above and beside it.
type browserState struct {
	cwd      string
	pathEdit string
	entries  []remotefs.Entry
	loading  bool
	err      error
	table    ui.ListState
	chosen   ui.Selection[string] // names in cwd
	row      int

	// sort is the order the user chose with a click on a column's header,
	// and applied the one the entries are in: they are sorted again as
	// the two differ.
	sort    ui.SortOrder
	applied ui.SortOrder

	// reveal is a name to choose in the table once cwd is read again: a
	// file just renamed or made.
	reveal string

	// seq numbers the folder asked for: a listing that comes back after a
	// later one was asked for is dropped, as it may be older than it.
	seq uint64

	tree     ui.OutlineState[string]
	treeRow  int
	subdirs  map[string][]string // the directories in each loaded directory
	treeBusy map[string]bool
}

// browserDir lists a folder of the panel's files, folders first and then
// by name, as remotefs.ReadDir does.
type browserDir func(ctx context.Context, dir string) ([]remotefs.Entry, error)

// init resets the panel for the first read of a session.
func (f *browserState) init() {
	f.subdirs = map[string][]string{}
	f.treeBusy = map[string]bool{}
	f.row = -1
	f.treeRow = -1
	f.tree.Open.Add("/")
}

// chosenNames are the names of the rows chosen in the table.
func (f *browserState) chosenNames() []string {
	var names []string
	for _, e := range f.entries {
		if f.chosen.Has(e.Name) {
			names = append(names, e.Name)
		}
	}
	return names
}

// entry is the entry of the given names when exactly one is, as the
// panel's context menus act on it.
func (f *browserState) entry(names []string) (remotefs.Entry, bool) {
	if len(names) != 1 {
		return remotefs.Entry{}, false
	}
	i := slices.IndexFunc(f.entries, func(e remotefs.Entry) bool { return e.Name == names[0] })
	if i < 0 {
		return remotefs.Entry{}, false
	}
	return f.entries[i], true
}

// sortEntries puts f.entries in the order the table asks: the folders
// first, as the server lists them, then by the column of the sort, and by
// name among the entries the column cannot tell apart. The row the cursor
// is on stays on the same file.
func (f *browserState) sortEntries() {
	f.applied = f.sort
	by, desc := f.sort.Column, f.sort.Descending
	var row string
	if f.row >= 0 && f.row < len(f.entries) {
		row = f.entries[f.row].Name
	}
	slices.SortStableFunc(f.entries, func(x, y remotefs.Entry) int {
		if x.IsDir() != y.IsDir() {
			if x.IsDir() {
				return -1
			}
			return 1
		}
		c := compareEntries(by, x, y)
		if desc {
			c = -c
		}
		if c == 0 {
			return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		}
		return c
	})
	if row == "" {
		return
	}
	if i := slices.IndexFunc(f.entries, func(e remotefs.Entry) bool { return e.Name == row }); i >= 0 {
		f.row = i
		f.table.ScrollIntoView(i)
	}
}

// browserOf is the file panel state of the files s shows.
func (s *Session) browserOf() *browserState {
	if s.isADB() {
		return &s.adbFiles.browserState
	}
	return &s.files.browserState
}

// browse lists dir in the panel of s.
func (a *App) browse(s *Session, dir string) {
	if s.isADB() {
		a.loadADBFiles(s, dir)
		return
	}
	a.openDir(s, dir)
}

// reloadSubdirs reads again the folders of the tree in dir.
func (a *App) reloadSubdirs(s *Session, dir string) {
	if s.isADB() {
		a.loadADBSubdirs(s, dir)
		return
	}
	a.loadSubdirs(s, dir)
}

// listFolder lists dir in the panel's table and makes it the folder shown.
// quiet leaves the panel as it was, with no error, when dir can't be read,
// as when it follows the shell into a folder SFTP can't open. loadSub
// reads the folders of the tree around dir.
func (a *App) listFolder(f *browserState, read browserDir, ctx context.Context, dir string, quiet bool, loadSub func(dir string)) {
	dir = path.Clean(dir)
	f.loading = true
	if !quiet {
		f.err = nil
		f.pathEdit = dir
	}
	f.seq++
	seq := f.seq
	go func() {
		entries, err := read(ctx, dir)
		a.post(func() {
			if f.seq != seq {
				return // a later folder was asked for
			}
			f.loading = false
			if err != nil {
				if !quiet {
					f.err = &dirError{dir, err}
					f.pathEdit = f.cwd
				}
				return
			}
			f.err = nil
			if dir != f.cwd {
				f.chosen.Clear()
				f.row = -1
			}
			f.cwd, f.pathEdit, f.entries = dir, dir, entries
			f.subdirs[dir] = dirNames(dir, entries) // the tree keeps its order
			f.sortEntries()
			if f.reveal != "" {
				if i := slices.IndexFunc(f.entries, func(e remotefs.Entry) bool { return e.Name == f.reveal }); i >= 0 {
					f.chosen.Clear()
					f.chosen.Add(f.reveal)
					f.row = i
					f.table.ScrollIntoView(i)
				}
			}
			f.reveal = ""
			// Open the tree down to the directory.
			for p := dir; ; p = path.Dir(p) {
				f.tree.Open.Add(p)
				if p == "/" {
					break
				}
			}
			for p := path.Dir(dir); p != "/" && f.subdirs[p] == nil; p = path.Dir(p) {
				loadSub(p)
			}
			if dir != "/" && f.subdirs["/"] == nil {
				loadSub("/")
			}
		})
	}()
}

// loadSubdirsInto reads the directories in dir, for the tree.
func (a *App) loadSubdirsInto(f *browserState, read browserDir, ctx context.Context, dir string) {
	if f.treeBusy[dir] {
		return
	}
	f.treeBusy[dir] = true
	go func() {
		entries, err := read(ctx, dir)
		a.post(func() {
			delete(f.treeBusy, dir)
			if err != nil {
				f.subdirs[dir] = []string{}
				return
			}
			f.subdirs[dir] = dirNames(dir, entries)
		})
	}()
}

// folderTree draws the tree of the folders the panel browses, and opens
// the one chosen. row adds what the panel does with a folder — a drop
// target, its menu — and open lists a folder.
func (a *App) folderTree(c *ui.Context, f *browserState, loadSub func(dir string), row func(ui.Element, string), open func(string)) {
	f.tree.List.Selected = &f.treeRow
	tree := ui.Outline(c, &f.tree, []string{"/"}, func(dir string) []string {
		subs, loaded := f.subdirs[dir]
		if !loaded {
			if f.tree.Open.Has(dir) {
				loadSub(dir)
			}
			return []string{}
		}
		if len(subs) == 0 {
			return nil
		}
		return subs
	}, func(dir string) {
		line := ui.Row(c).Gap(5).Grow(1).Radius(4)
		row(line, dir)
		line.Children(func() {
			name := path.Base(dir)
			// "/" is a folder like any other, not a workspace's root,
			// which themes give an icon of its own.
			a.fileIcon(c, name, true, f.tree.Open.Has(dir), false)
			t := ui.Text(c, name).SingleLine()
			if dir == f.cwd {
				t.Bold()
			}
		})
	}).Fill()
	if tree.Changed() {
		if dir := f.tree.Item(f.treeRow); dir != "" && dir != f.cwd {
			open(dir)
		}
	}
}

// browserTable draws the panel's table, in the order chosen, and hands
// the entry opened to submit. decorate adds what the panel does with the
// table itself — its menu, its keys — and zone is where files are
// dropped to upload them.
func (a *App) browserTable(c *ui.Context, f *browserState, columns []ui.TableColumn, zone ui.Element, submit func(remotefs.Entry), decorate func(ui.Element)) {
	f.table.Sort = &f.sort
	if f.sort != f.applied {
		f.sortEntries()
	}
	entries := f.entries
	f.table.Selected = &f.row
	f.table.Selection = &f.chosen
	f.table.Key = func(i int) any { return entries[i].Name }
	f.table.Label = func(i int) string { return entries[i].Name }
	over := zone.FileDragOver() && f.cwd != ""
	zone.Children(func() {
		t := ui.Table(c, &f.table, columns, len(entries), func(row, col int) {
			e := entries[row]
			a.fileCell(c, e, col, f.chosen.Has(e.Name))
		}).Grow(1)
		// A double click or Enter opens a folder, or a file.
		if t.Submitted() && f.row >= 0 && f.row < len(entries) {
			submit(entries[f.row])
		}
		if decorate != nil {
			decorate(t)
		}
		if over {
			ui.Text(c, locales.DropToUpload(f.cwd)).Padding(6, 12).Radius(6).
				Background(theme.Accent).TextColor(theme.AccentText).
				Attach(ui.AnchorBottom, ui.AnchorBottom).Bottom(12)
		}
	})
}

// fileCell draws one cell of the panel's table: the name with its icon,
// the size, the kind, the time, the permissions and the owner.
func (a *App) fileCell(c *ui.Context, e remotefs.Entry, col int, chosen bool) {
	switch col {
	case 0:
		// A chosen row keeps the table's text color, which shows on
		// the accent.
		ui.Row(c).Gap(6).Children(func() {
			a.entryIcon(c, e)
			name := ui.Text(c, e.Name).SingleLine()
			if e.Broken && !chosen {
				name.TextColor(theme.Danger)
			}
			if e.Target != "" {
				// As ls -l shows it; cut before the name.
				target := ui.Text(c, "→ "+e.Target).SingleLine().Shrink(1).MinWidth(0)
				if !chosen {
					target.TextColor(theme.TextMuted)
				}
			}
		})
	case 1:
		// A broken link has no file to measure.
		if !e.IsDir() && !e.Broken {
			ui.Text(c, formatSize(e.Size))
		}
	case 2:
		ui.Text(c, entryKind(e)).SingleLine()
	case 3:
		ui.Text(c, e.ModTime.Format("2006/01/02 15:04")).SingleLine()
	case 4:
		ui.Text(c, e.Mode.Perm().String()).Font("monospace")
	case 5:
		ui.Text(c, fmt.Sprintf("%d/%d", e.UID, e.GID))
	}
}
