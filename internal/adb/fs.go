package adb

// The device's files, over its sync service: what the files tab lists and
// copies, and the shell for what sync has no word for.

import (
	"context"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/pgaskin/go-adb/adblib/adbsync"
)

// Entry is a file of the device, as the files tab shows it.
type Entry struct {
	Name    string
	Dir     bool
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	// Link is a symbolic link. The sync service does not say where it
	// points, so it is never known to be broken.
	Link bool
}

// sync is the client of the device's sync service.
func (c *Conn) sync() *adbsync.Client { return c.syncClient }

// ReadDir lists dir.
func (c *Conn) ReadDir(ctx context.Context, dir string) ([]Entry, error) {
	fi, err := c.sync().Stat(ctx, dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, &pathError{dir, "not a directory"}
	}
	des, err := c.sync().ReadDir(ctx, dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		e := Entry{Name: de.Name(), Dir: de.IsDir(), Mode: de.Type()}
		if fi, err := de.Info(); err == nil {
			e.Size, e.ModTime, e.Mode = fi.Size(), fi.ModTime(), fi.Mode()
			e.Dir = e.Mode.IsDir()
			e.Link = e.Mode&fs.ModeSymlink != 0
		}
		out = append(out, e)
	}
	// Folders first, then by name, as the files of a server are.
	sortEntries(out)
	return out, nil
}

// Stat describes the file or folder at name.
func (c *Conn) Stat(ctx context.Context, name string) (Entry, error) {
	fi, err := c.sync().Stat(ctx, name)
	if err != nil {
		return Entry{}, err
	}
	return entryOf(path.Base(name), fi), nil
}

// entryOf is a file of the device from what the sync service said of it.
func entryOf(name string, fi fs.FileInfo) Entry {
	e := Entry{Name: name, Dir: fi.IsDir(), Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime()}
	e.Link = e.Mode&fs.ModeSymlink != 0
	return e
}

// OpenFile reads a file of the device.
func (c *Conn) OpenFile(ctx context.Context, name string) (io.ReadCloser, error) {
	return c.sync().Open(ctx, name)
}

// CreateFile writes a file of the device.
func (c *Conn) CreateFile(ctx context.Context, name string) (io.WriteCloser, error) {
	return c.sync().Create(ctx, name, 0o644, time.Now())
}

// Mkdir makes dir, and the folders above it.
func (c *Conn) Mkdir(ctx context.Context, dir string) error {
	_, err := c.Run(ctx, "mkdir -p "+shellQuote(dir))
	return err
}

// Remove removes a file or a folder, and what is in it.
func (c *Conn) Remove(ctx context.Context, name string) error {
	_, err := c.Run(ctx, "rm -rf "+shellQuote(name))
	return err
}

// Rename moves a file or a folder.
func (c *Conn) Rename(ctx context.Context, from, to string) error {
	_, err := c.Run(ctx, "mv "+shellQuote(from)+" "+shellQuote(to))
	return err
}

// CleanDir is a path of the device as the tab keeps it: absolute, without
// the ".." and "." of a path typed by hand.
func CleanDir(dir string) string {
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}
	return path.Clean(dir)
}

// pathError is a failure about one path.
type pathError struct{ path, msg string }

func (e *pathError) Error() string { return e.path + ": " + e.msg }

// sortEntries puts the folders first and then the names in order.
func sortEntries(es []Entry) {
	for i := 1; i < len(es); i++ {
		for j := i; j > 0; j-- {
			a, b := es[j-1], es[j]
			if a.Dir != b.Dir {
				if !a.Dir {
					es[j-1], es[j] = b, a
					continue
				}
				break
			}
			if strings.ToLower(a.Name) <= strings.ToLower(b.Name) {
				break
			}
			es[j-1], es[j] = b, a
		}
	}
}
