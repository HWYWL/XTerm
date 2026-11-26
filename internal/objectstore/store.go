// Package objectstore reads and writes the files of an object storage or a
// directory, behind one interface: a bucket lists its keys by prefixes, and
// a directory its files by folders, and the panel of the app shows both the
// same way.
package objectstore

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// Entry is a folder or an object of a storage, as the panel shows it.
type Entry struct {
	Key     string // the object's key, or the folder's prefix without its slash
	Name    string
	Size    int64
	ModTime time.Time
	Dir     bool
}

// Store is a storage the panel browses: a directory of this machine, a
// bucket of an S3 server, or a container of Azure.
type Store interface {
	// Kind is the provider's name, for the panel's title.
	Kind() string
	// Home is the folder the panel opens on.
	Home() string
	// List returns what dir holds, its folders and its objects, without
	// their descendants. dir is a prefix, without a slash at its end.
	List(ctx context.Context, dir string) ([]Entry, error)
	// Get opens an object for reading.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Put writes an object, of size bytes, which the reader holds from
	// its start; it is read twice, once to sign it, and progress, when it
	// is not nil, is told how many bytes have been written as they are.
	Put(ctx context.Context, key string, r io.ReadSeeker, size int64, contentType string, progress Progress) error
	// Delete removes an object, or a folder and what it holds.
	Delete(ctx context.Context, key string, dir bool) error
	// Mkdir makes a folder; object storages have none, and say so.
	Mkdir(ctx context.Context, key string) error
	// Stat returns one entry: a folder's, or an object's.
	Stat(ctx context.Context, key string) (Entry, error)
	// Probe asks for the root of the storage, as a HEAD of the bucket or a
	// look at the directory: whether it is reachable, and how fast.
	Probe(ctx context.Context) error
}

// Progress is told how many bytes of a Put have been written, as they are
// written: the sends of the body, not the pass that signs it.
type Progress func(done int64)

// countingReader tells progress how much of r has been read.
type countingReader struct {
	r      io.Reader
	done   int64
	report Progress
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	if n > 0 {
		c.done += int64(n)
		c.report(c.done)
	}
	return n, err
}

// ErrNoFolders is what an object storage answers to Mkdir: its keys are
// flat, and a folder is a prefix of them.
var ErrNoFolders = fmt.Errorf("object stores have no folders")

// ErrNotFound is what a storage answers for a key it does not hold.
var ErrNotFound = fmt.Errorf("not found")

// CleanKey is a key as the panel keeps it: no slashes around it, one
// between its parts, and none at all for the root.
func CleanKey(key string) string {
	key = strings.Trim(key, "/")
	if key == "" {
		return ""
	}
	return path.Clean("/" + key)[1:]
}

// JoinKey is a key under dir, as the panel names it.
func JoinKey(dir, name string) string {
	name = strings.Trim(name, "/")
	if dir == "" {
		return CleanKey(name)
	}
	return CleanKey(dir + "/" + name)
}

// BaseKey is the name of the last part of a key.
func BaseKey(key string) string {
	key = CleanKey(key)
	if key == "" {
		return "/"
	}
	return path.Base(key)
}

// rooted is a store seen from inside a folder of it: the keys the panel
// shows are under root, and Home is the root itself.
type rooted struct {
	Store
	root string // without slashes around it
}

// WithRoot shows s from inside root, as the host's Root field asks.
func WithRoot(s Store, root string) Store {
	if root = strings.Trim(root, "/"); root == "" {
		return s
	}
	return &rooted{Store: s, root: root}
}

// inner is the key of the storage that key names.
func (r *rooted) inner(key string) string {
	key = CleanKey(key)
	if key == "" {
		return r.root
	}
	return r.root + "/" + key
}

// outer is key without the root before it.
func (r *rooted) outer(key string) string {
	return CleanKey(strings.TrimPrefix(CleanKey(key), r.root))
}

func (r *rooted) Home() string { return "" }

func (r *rooted) List(ctx context.Context, dir string) ([]Entry, error) {
	entries, err := r.Store.List(ctx, r.inner(dir))
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Key = r.outer(entries[i].Key)
	}
	return entries, nil
}

func (r *rooted) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return r.Store.Get(ctx, r.inner(key))
}

func (r *rooted) Put(ctx context.Context, key string, f io.ReadSeeker, size int64, contentType string, progress Progress) error {
	return r.Store.Put(ctx, r.inner(key), f, size, contentType, progress)
}

func (r *rooted) Delete(ctx context.Context, key string, dir bool) error {
	return r.Store.Delete(ctx, r.inner(key), dir)
}

func (r *rooted) Mkdir(ctx context.Context, key string) error {
	return r.Store.Mkdir(ctx, r.inner(key))
}

func (r *rooted) Probe(ctx context.Context) error { return r.Store.Probe(ctx) }

func (r *rooted) Stat(ctx context.Context, key string) (Entry, error) {
	e, err := r.Store.Stat(ctx, r.inner(key))
	if err != nil {
		return e, err
	}
	e.Key = r.outer(e.Key)
	e.Name = BaseKey(e.Key)
	return e, nil
}
