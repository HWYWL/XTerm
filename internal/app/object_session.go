package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/objectstore"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// objectTimeout bounds connecting to a storage and each of its requests, as
// a server that does not answer must not hold the panel.
const objectTimeout = 20 * time.Second

// objectState is the file panel of an object storage session: the folder
// shown, its entries, what is chosen, and what is under way.
type objectState struct {
	cwd      string
	pathEdit string
	entries  []objectstore.Entry
	loading  bool
	err      error
	table    ui.ListState
	chosen   ui.Selection[string] // keys in cwd
	row      int
	busy     string // what the panel is doing, as "Uploading notes.txt"

	// transfers are the files going up and down, newest last
	// (objecttransfers.go).
	transfers   []*objectTransfer
	transferSeq int
}

// init resets the panel for a storage just connected.
func (f *objectState) init() {
	f.cwd, f.pathEdit, f.entries = "", "", nil
	f.loading, f.err, f.row, f.busy = false, nil, -1, ""
	f.transfers, f.transferSeq = nil, 0
	f.chosen.Clear()
}

// chosenKeys are the keys of the rows chosen in the table.
func (f *objectState) chosenKeys() []string {
	var keys []string
	for _, e := range f.entries {
		if f.chosen.Has(e.Key) {
			keys = append(keys, e.Key)
		}
	}
	return keys
}

// entry returns the one row chosen, when exactly one is.
func (f *objectState) entry(keys []string) (objectstore.Entry, bool) {
	if len(keys) != 1 {
		return objectstore.Entry{}, false
	}
	for _, e := range f.entries {
		if e.Key == keys[0] {
			return e, true
		}
	}
	return objectstore.Entry{}, false
}

// sortEntries puts the folders first and the entries by name, as the file
// panel does.
func (f *objectState) sortEntries() {
	sort.SliceStable(f.entries, func(i, j int) bool {
		x, y := f.entries[i], f.entries[j]
		if x.Dir != y.Dir {
			return x.Dir
		}
		return strings.ToLower(x.Name) < strings.ToLower(y.Name)
	})
}

// objectOptions is how the host's storage is opened, with the secret key.
func objectOptions(h *config.Host, secret string) objectstore.Options {
	o := h.Object
	if o == nil {
		o = &config.ObjectSettings{}
	}
	return objectstore.Options{
		Provider: o.Provider, Endpoint: o.Endpoint, Region: o.Region, Bucket: o.Bucket,
		AccessKey: o.AccessKey, SecretKey: secret, Root: h.ObjectRoot(),
		LocalPath: o.LocalPath, PathStyle: o.PathStyle, Insecure: o.Insecure,
		Timeout: objectTimeout,
	}
}

// connectObject opens the storage of a host and reads its first folder, on
// a goroutine: the panel shows what it holds, and its errors, once it has.
func (a *App) connectObject(s *Session, secret string) {
	if s.cancel != nil {
		s.cancel()
	}
	s.status, s.err = statusConnecting, nil
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.ctx = cancel, ctx
	h := *s.Host
	opts := objectOptions(&h, secret)
	go func() {
		store, err := objectstore.Open(opts)
		var entries []objectstore.Entry
		start := time.Now()
		if err == nil {
			rctx, cancel := context.WithTimeout(ctx, objectTimeout)
			entries, err = store.List(rctx, store.Home())
			cancel()
		}
		a.post(func() {
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				s.status, s.err = statusFailed, err
				a.assetLogin(false)
				return
			}
			s.object = store
			s.objects.init()
			s.objects.cwd, s.objects.pathEdit, s.objects.entries = store.Home(), objectPath(store.Home()), entries
			s.objects.sortEntries()
			s.status, s.err = statusConnected, nil
			a.assetLogin(true)
			a.noteConnected(s.Host)
			// Its storage's page is what one wants next, as an SSH tab's
			// server page is.
			a.showMonitorOfConnection()
			a.startObjectMonitor(s, ctx, store)
			s.noteObjectCall("List", start, err)
		})
	}()
}

// objectOpen lists dir in the session's panel, and makes it the folder
// shown.
func (a *App) objectOpen(s *Session, dir string) {
	store := s.object
	if store == nil {
		return
	}
	dir = objectstore.CleanKey(dir)
	f := &s.objects
	f.loading, f.err = true, nil
	f.pathEdit = objectPath(dir)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), objectTimeout)
		defer cancel()
		start := time.Now()
		entries, err := store.List(ctx, dir)
		a.post(func() {
			s.noteObjectCall("List", start, err)
			if s.object != store {
				return
			}
			f.loading = false
			if err != nil {
				f.err = err
				f.pathEdit = f.cwd
				return
			}
			f.cwd, f.pathEdit, f.entries = dir, objectPath(dir), entries
			f.sortEntries()
			f.chosen.Clear()
			f.row = -1
		})
	}()
}

// objectUpload asks for files of this machine and writes them into dir of
// the session's storage, each under its name.
func (a *App) objectUpload(s *Session, dir string) {
	if s.object == nil {
		return
	}
	title := locales.UploadFiles()
	go func() {
		paths, err := a.plat.OpenFiles(title, false)
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() { a.objectUploadPaths(s, dir, paths) })
	}()
}

// objectUploadPaths writes the files of this machine into dir of the
// session's storage, each under its name: what the picker chose, and what
// was dropped on the panel.
func (a *App) objectUploadPaths(s *Session, dir string, paths []string) {
	store := s.object
	if store == nil {
		return
	}
	go func() {
		for _, p := range paths {
			name := filepath.Base(p)
			f, err := os.Open(p)
			if err != nil {
				msg := err
				a.post(func() { a.notify(fmt.Sprintf(locales.ObjectUploadFailed(), msg)) })
				continue
			}
			info, err := f.Stat()
			if err != nil {
				f.Close()
				msg := err
				a.post(func() { a.notify(fmt.Sprintf(locales.ObjectUploadFailed(), msg)) })
				continue
			}
			key := objectstore.JoinKey(dir, name)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			t := &objectTransfer{
				upload: true, name: name, where: "/" + key,
				total: info.Size(), startAt: time.Now(), cancel: cancel,
			}
			a.post(func() {
				s.addObjectTransfer(t)
				s.objects.busy = fmt.Sprintf(locales.ObjectUploading(), name)
			})
			start := time.Now()
			uploadErr := store.Put(ctx, key, f, info.Size(), "", func(done int64) { t.report(s, done) })
			cancel()
			f.Close()
			a.post(func() {
				s.noteObjectCall("Put", start, uploadErr)
				t.finish(s, uploadErr)
				s.objects.busy = ""
				if uploadErr != nil {
					a.notify(fmt.Sprintf(locales.ObjectUploadFailed(), uploadErr))
				} else {
					a.notify(fmt.Sprintf(locales.ObjectUploaded(), name))
				}
				a.objectOpen(s, s.objects.cwd)
			})
		}
	}()
}

// objectDownload asks where to save one object and writes it there.
func (a *App) objectDownload(s *Session, e objectstore.Entry) {
	store := s.object
	if store == nil || e.Dir {
		return
	}
	name := e.Name
	dir := a.plat.DownloadsDir()
	go func() {
		target, err := a.plat.SaveFile(WindowMain, locales.Download(), filepath.Join(dir, name), "", nil)
		if err != nil || target == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		t := &objectTransfer{
			name: name, where: target, total: e.Size, startAt: time.Now(), cancel: cancel,
		}
		a.post(func() {
			s.addObjectTransfer(t)
			s.objects.busy = fmt.Sprintf(locales.ObjectDownloading(), name)
		})
		start := time.Now()
		_, getErr := downloadObject(ctx, store, e.Key, target, s, t)
		cancel()
		a.post(func() {
			s.noteObjectCall("Get", start, getErr)
			t.finish(s, getErr)
			s.objects.busy = ""
			if getErr != nil {
				a.notify(fmt.Sprintf(locales.ObjectDownloadFailed(), getErr))
				return
			}
			a.notify(fmt.Sprintf(locales.ObjectDownloaded(), name))
		})
	}()
}

// downloadObject writes the storage's key into the file at target, and
// says how many bytes it carried; the transfer hears of them as they come.
func downloadObject(ctx context.Context, store objectstore.Store, key, target string, s *Session, t *objectTransfer) (int64, error) {
	r, err := store.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(target)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, &progressReader{r: r, s: s, t: t})
	if err != nil {
		f.Close()
		return n, err
	}
	return n, f.Close()
}

// objectDelete removes the keys chosen, with what folders under them hold.
func (a *App) objectDelete(s *Session, entries []objectstore.Entry) {
	store := s.object
	if store == nil || len(entries) == 0 {
		return
	}
	a.post(func() { s.objects.busy = locales.ObjectDeleting() })
	go func() {
		var failure error
		for _, e := range entries {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			start := time.Now()
			err := store.Delete(ctx, e.Key, e.Dir)
			cancel()
			a.post(func() { s.noteObjectCall("Delete", start, err) })
			if err != nil {
				failure = err
				break
			}
		}
		a.post(func() {
			s.objects.busy = ""
			if failure != nil {
				a.notify(fmt.Sprintf(locales.ObjectDeleteFailed(), failure))
			} else {
				a.notify(fmt.Sprintf(locales.ObjectDeleted(), len(entries)))
			}
			a.objectOpen(s, s.objects.cwd)
		})
	}()
}

// objectMkdir makes a folder in the session's storage, as the consoles do.
func (a *App) objectMkdir(s *Session, dir, name string) {
	store := s.object
	if store == nil {
		return
	}
	key := objectstore.JoinKey(dir, strings.Trim(strings.TrimSpace(name), "/"))
	if key == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), objectTimeout)
		err := store.Mkdir(ctx, key)
		cancel()
		a.post(func() {
			if err != nil {
				a.notify(fmt.Sprintf(locales.ObjectMkdirFailed(), err))
				return
			}
			a.objectOpen(s, s.objects.cwd)
		})
	}()
}

// objectUp is the folder above dir, or the root.
func objectUp(dir string) string {
	dir = objectstore.CleanKey(dir)
	if dir == "" {
		return ""
	}
	if i := strings.LastIndex(dir, "/"); i >= 0 {
		return dir[:i]
	}
	return ""
}

// objectPath is dir, or "/" for the root.
func objectPath(dir string) string {
	dir = objectstore.CleanKey(dir)
	if dir == "" {
		return "/"
	}
	return "/" + dir
}
