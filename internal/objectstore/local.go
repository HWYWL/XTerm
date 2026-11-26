package objectstore

import (
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Local is a directory of this machine, browsed as the storages are: its
// folders and files, under the root the host gives.
type Local struct {
	root string // absolute path
}

// OpenLocal opens dir, which must exist, as a storage.
func OpenLocal(dir string) (*Local, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "open", Path: abs, Err: os.ErrInvalid}
	}
	return &Local{root: abs}, nil
}

// Kind is the storage's name.
func (l *Local) Kind() string { return "Local" }

// Home is the root directory, as the panel names it.
func (l *Local) Home() string { return "" }

// Path is the file of a key.
func (l *Local) Path(key string) string {
	key = CleanKey(key)
	if key == "" {
		return l.root
	}
	return filepath.Join(l.root, filepath.FromSlash(key))
}

// List reads a folder.
func (l *Local) List(ctx context.Context, dir string) ([]Entry, error) {
	dir = CleanKey(dir)
	names, err := os.ReadDir(l.Path(dir))
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(names))
	for _, n := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := n.Info()
		if err != nil {
			continue // gone since the listing began
		}
		key := JoinKey(dir, n.Name())
		entries = append(entries, Entry{
			Key: key, Name: n.Name(), Size: info.Size(), ModTime: info.ModTime(), Dir: n.IsDir(),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

// Get opens a file.
func (l *Local) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return os.Open(l.Path(key))
}

// Put writes a file, replacing it.
func (l *Local) Put(ctx context.Context, key string, r io.ReadSeeker, size int64, contentType string, progress Progress) error {
	p := l.Path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	buf := make([]byte, 64<<10)
	var done int64
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			done += int64(n)
			if progress != nil {
				progress(done)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	return f.Close()
}

// Delete removes a file, or a folder and what it holds.
func (l *Local) Delete(ctx context.Context, key string, dir bool) error {
	return os.RemoveAll(l.Path(key))
}

// Mkdir makes a folder.
func (l *Local) Mkdir(ctx context.Context, key string) error {
	return os.MkdirAll(l.Path(key), 0o755)
}

// Stat reads one entry.
func (l *Local) Stat(ctx context.Context, key string) (Entry, error) {
	info, err := os.Stat(l.Path(key))
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Key: CleanKey(key), Name: path.Base(CleanKey(key)), Size: info.Size(),
		ModTime: info.ModTime(), Dir: info.IsDir(),
	}, nil
}

// Probe looks at the directory.
func (l *Local) Probe(ctx context.Context) error {
	_, err := os.Stat(l.root)
	return err
}

// SetModTime sets a file's time, as an upload of it would keep.
func (l *Local) SetModTime(key string, t time.Time) error {
	return os.Chtimes(l.Path(key), t, t)
}
