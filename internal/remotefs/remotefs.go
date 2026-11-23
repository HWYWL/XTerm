// Package remotefs lists, uploads, downloads, renames, creates and
// deletes files over SFTP.
package remotefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// NewClient starts SFTP on an SSH connection, with transfers that send and
// fetch several chunks at once, which is what makes them fast on links
// with a long round trip.
func NewClient(client *ssh.Client) (*sftp.Client, error) {
	return sftp.NewClient(client, sftp.UseConcurrentWrites(true), sftp.UseConcurrentReads(true))
}

// Entry is a file or directory on the server.
type Entry struct {
	Name string
	// Size is the size of the file, or of the file a link points to.
	Size    int64
	Mode    fs.FileMode // of the target, for a link to a directory
	ModTime time.Time
	UID     uint32
	GID     uint32
	Link    bool
	// Target is where a link points, as readlink gives it: relative or
	// absolute, not resolved. Empty when it can't be read.
	Target string
	// Broken is a link whose target is not there, or a loop of links. A
	// target the user may not look at is not broken.
	Broken bool
}

func (e Entry) IsDir() bool { return e.Mode.IsDir() }

// linkWorkers is how many links ReadDir resolves at once. Each takes two
// round trips, and SFTP answers requests in parallel, so a directory with
// hundreds of links doesn't wait for them one after another.
const linkWorkers = 16

// ReadDir lists dir, directories first, then by name.
func ReadDir(c *sftp.Client, dir string) ([]Entry, error) {
	infos, err := c.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(infos))
	var links []int
	for _, fi := range infos {
		e := Entry{Name: fi.Name(), Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime()}
		if st, ok := fi.Sys().(*sftp.FileStat); ok {
			e.UID, e.GID = st.UID, st.GID
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			e.Link = true
			links = append(links, len(entries))
		}
		entries = append(entries, e)
	}
	jobs := make(chan *Entry)
	var wg sync.WaitGroup
	for range min(linkWorkers, len(links)) {
		wg.Go(func() {
			for e := range jobs {
				resolveLink(c, path.Join(dir, e.Name), e)
			}
		})
	}
	for _, i := range links {
		jobs <- &entries[i]
	}
	close(jobs)
	wg.Wait()
	slices.SortFunc(entries, func(x, y Entry) int {
		if x.IsDir() != y.IsDir() {
			if x.IsDir() {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
	})
	return entries, nil
}

// resolveLink fills in where the link e at p points, and what is there.
func resolveLink(c *sftp.Client, p string, e *Entry) {
	if target, err := c.ReadLink(p); err == nil {
		e.Target = target
	}
	target, err := c.Stat(p)
	switch {
	case err != nil:
		e.Broken = !errors.Is(err, fs.ErrPermission)
	case target.IsDir():
		e.Mode = target.Mode()
	default:
		e.Size = target.Size()
	}
}

// RemoteExisting returns which of names are in the remote directory dir.
func RemoteExisting(c *sftp.Client, dir string, names []string) []string {
	var found []string
	for _, n := range names {
		if _, err := c.Lstat(path.Join(dir, n)); err == nil {
			found = append(found, n)
		}
	}
	return found
}

// LocalExisting returns which of names are in the local directory dir.
func LocalExisting(dir string, names []string) []string {
	var found []string
	for _, n := range names {
		if _, err := os.Lstat(filepath.Join(dir, n)); err == nil {
			found = append(found, n)
		}
	}
	return found
}

// LocalSize is the number of bytes of the files in p, a file or a
// directory. Links to directories are not followed, as Upload skips them.
func LocalSize(p string) (int64, error) {
	var total int64
	err := filepath.WalkDir(p, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fi, err := os.Stat(name)
		if err == nil && fi.Mode().IsRegular() {
			total += fi.Size()
		}
		return nil
	})
	return total, err
}

// RemoteSize is the number of bytes of the files in p, a file or a
// directory, as Download would fetch them.
func RemoteSize(c *sftp.Client, p string) (int64, error) {
	fi, err := c.Stat(p)
	if err != nil {
		return 0, err
	}
	if !fi.IsDir() {
		return fi.Size(), nil
	}
	var total atomic.Int64
	err = walkRemote(c, p, func(_ string, fi fs.FileInfo) error {
		total.Add(fi.Size())
		return nil
	})
	return total.Load(), err
}

// remoteWalkers is how many directories a walk lists at once. Listing one
// is a round trip, so a tree of many directories is otherwise bound by
// them, which is what makes a folder of many files slow to measure and to
// fetch on a link with a long round trip.
const remoteWalkers = 8

// walkRemote calls file for each regular file under the directory dir,
// with its path. Links to directories are not followed, which keeps a
// link to a parent from looping.
//
// The directories are listed remoteWalkers at a time, so file is called
// from several goroutines, in no order, and must be safe for that.
func walkRemote(c *sftp.Client, dir string, file func(p string, fi fs.FileInfo) error) error {
	type found struct {
		p  string
		fi fs.FileInfo
	}
	var (
		mu       sync.Mutex
		cond     = sync.NewCond(&mu)
		queue    = []string{dir}
		pending  = 1 // directories queued or being listed
		firstErr error
	)
	fail := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	var wg sync.WaitGroup
	wg.Add(remoteWalkers)
	for range remoteWalkers {
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				for len(queue) == 0 && pending > 0 && firstErr == nil {
					cond.Wait()
				}
				if firstErr != nil || len(queue) == 0 {
					mu.Unlock()
					return
				}
				d := queue[0]
				queue = queue[1:]
				mu.Unlock()

				infos, err := c.ReadDir(d)
				var files []found
				if err == nil {
					for _, fi := range infos {
						p := path.Join(d, fi.Name())
						if fi.Mode()&fs.ModeSymlink != 0 {
							target, err := c.Stat(p)
							if err != nil || target.IsDir() {
								continue
							}
							fi = target
						}
						switch {
						case fi.IsDir():
							mu.Lock()
							queue = append(queue, p)
							pending++
							mu.Unlock()
						case fi.Mode().IsRegular():
							files = append(files, found{p, fi})
						}
					}
				}
				mu.Lock()
				if err != nil {
					fail(fmt.Errorf("%s: %w", d, err))
				}
				pending--
				cond.Broadcast()
				mu.Unlock()
				if err != nil {
					return
				}
				for _, f := range files {
					if err := file(f.p, f.fi); err != nil {
						mu.Lock()
						fail(err)
						cond.Broadcast()
						mu.Unlock()
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	return firstErr
}

// Progress is told the bytes copied so far, by the goroutines copying the
// files of a folder, one call at a time.
type Progress func(done int64)

// counter adds up the bytes copied by the goroutines of a transfer, and
// tells progress, one call at a time, as a transfer of a folder copies
// several files at once (transferWorkers).
type counter struct {
	mu       sync.Mutex
	done     int64
	progress Progress
}

func (c *counter) add(n int) {
	if n <= 0 {
		return
	}
	c.mu.Lock()
	c.done += int64(n)
	done := c.done
	if c.progress != nil {
		c.progress(done)
	}
	c.mu.Unlock()
}

// countingReader counts what is read, and stops when the context ends.
// Its Size lets sftp write several chunks at once.
type countingReader struct {
	ctx  context.Context
	r    io.Reader
	size int64
	*counter
}

func (r *countingReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	r.add(n)
	return n, err
}

func (r *countingReader) Size() int64 { return r.size }

type countingWriter struct {
	ctx context.Context
	w   io.Writer
	*counter
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.w.Write(p)
	w.add(n)
	return n, err
}

// transferWorkers is how many files of a folder a transfer copies at once.
// A folder of many small files is bound by the round trips of each file
// (open, write, close, chmod), which side by side files overlap, not by
// the bandwidth of the link; each file still sends several chunks at once
// (NewClient).
const transferWorkers = 8

// pair is a file to copy: where it is and where it goes.
type pair struct {
	src, dst string
	fi       fs.FileInfo
}

// eachFile runs copy over files with up to transferWorkers goroutines,
// stopping the others at the first error and when ctx ends. It returns
// the context's error when the caller stopped it, and else the first
// error of a file.
func eachFile(ctx context.Context, files []pair, copy func(ctx context.Context, f pair) error) error {
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu   sync.Mutex
		werr error
		wg   sync.WaitGroup
	)
	jobs := make(chan pair)
	for range min(transferWorkers, len(files)) {
		wg.Go(func() {
			for f := range jobs {
				if err := copy(ctx, f); err != nil {
					mu.Lock()
					if werr == nil {
						werr = err
					}
					mu.Unlock()
					cancel() // the others stop at their next chunk
					return
				}
			}
		})
	}
feed:
	for _, f := range files {
		select {
		case jobs <- f:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if werr == nil {
		return parent.Err() // the caller stopped it
	}
	return werr
}

// Upload copies the local file or directory src into the remote directory
// dstDir, as dstDir/<name of src>, replacing files that are there. A
// directory's files go transferWorkers at a time.
func Upload(ctx context.Context, c *sftp.Client, src, dstDir string, progress Progress) error {
	cnt := &counter{progress: progress}
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	dst := path.Join(dstDir, filepath.Base(src))
	if !fi.IsDir() {
		return uploadFile(ctx, c, src, dst, fi, cnt)
	}
	if err := c.MkdirAll(dst); err != nil {
		return err
	}
	var files []pair
	err = filepath.WalkDir(src, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, name)
		if err != nil {
			return err
		}
		target := path.Join(dst, filepath.ToSlash(rel))
		if d.IsDir() {
			if name == src {
				return nil // dst, above
			}
			return c.MkdirAll(target)
		}
		fi, err := os.Stat(name)
		if err != nil || !fi.Mode().IsRegular() {
			return nil // a broken link, a device, a link to a directory
		}
		files = append(files, pair{src: name, dst: target, fi: fi})
		return nil
	})
	if err != nil {
		return err
	}
	return eachFile(ctx, files, func(ctx context.Context, f pair) error {
		return uploadFile(ctx, c, f.src, f.dst, f.fi, cnt)
	})
}

func uploadFile(ctx context.Context, c *sftp.Client, src, dst string, fi fs.FileInfo, cnt *counter) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := c.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("%s: %w", dst, err)
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			c.Remove(dst) // what was written is a part of the file
		}
	}()
	if _, err = out.ReadFrom(&countingReader{ctx: ctx, r: in, size: fi.Size(), counter: cnt}); err != nil {
		return err
	}
	c.Chmod(dst, fi.Mode().Perm()) // best effort: some servers refuse it
	return nil
}

// Download copies the remote file or directory src into the local
// directory dstDir, as dstDir/<name of src>, replacing files that are
// there, and returns the local path. A directory's files go
// transferWorkers at a time.
func Download(ctx context.Context, c *sftp.Client, src, dstDir string, progress Progress) (string, error) {
	cnt := &counter{progress: progress}
	fi, err := c.Stat(src)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dstDir, path.Base(src))
	if !fi.IsDir() {
		return dst, downloadFile(ctx, c, src, dst, fi, cnt)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return dst, err
	}
	// walkRemote calls this from several goroutines.
	var filesMu sync.Mutex
	var files []pair
	err = walkRemote(c, src, func(p string, fi fs.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, strings.TrimSuffix(src, "/")+"/")
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if !isInside(dst, target) {
			return fmt.Errorf("unsafe name %q", rel)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		filesMu.Lock()
		files = append(files, pair{src: p, dst: target, fi: fi})
		filesMu.Unlock()
		return nil
	})
	if err != nil {
		return dst, err
	}
	return dst, eachFile(ctx, files, func(ctx context.Context, f pair) error {
		return downloadFile(ctx, c, f.src, f.dst, f.fi, cnt)
	})
}

// isInside reports whether p is in dir, as a name the server sends must
// be, so that it cannot write elsewhere.
func isInside(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func downloadFile(ctx context.Context, c *sftp.Client, src, dst string, fi fs.FileInfo, cnt *counter) (err error) {
	in, err := c.Open(src)
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dst)
		}
	}()
	if _, err = in.WriteTo(&countingWriter{ctx: ctx, w: out, counter: cnt}); err != nil {
		return err
	}
	if perm := fi.Mode().Perm(); perm&0o111 != 0 {
		os.Chmod(dst, 0o755) // keep scripts and programs runnable
	}
	return nil
}

// IsCanceled reports whether err is a transfer stopped by its context.
func IsCanceled(err error) bool { return errors.Is(err, context.Canceled) }
