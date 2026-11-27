package app

// How the file panel's dialogs act on the files a session shows: a
// server's over SFTP (files.go) or a device's over ADB (adbfiles.go).

import (
	"context"
	"path"

	"finallyshell/internal/adb"
	"finallyshell/internal/remotefs"

	"github.com/pkg/sftp"
)

// fileBackend is the files a session's panel browses, as its rename, new
// folder, new file and delete dialogs see them.
type fileBackend interface {
	rename(ctx context.Context, dir, old, name string) (string, error)
	mkdir(ctx context.Context, dir, name string) (string, error)
	create(ctx context.Context, dir, name string) (string, error)
	removeAll(ctx context.Context, p string) error
}

// sftpFiles is a server's files, over SFTP.
type sftpFiles struct{ sc *sftp.Client }

func (b sftpFiles) rename(ctx context.Context, dir, old, name string) (string, error) {
	return remotefs.Rename(b.sc, path.Join(dir, old), name)
}

func (b sftpFiles) mkdir(ctx context.Context, dir, name string) (string, error) {
	return remotefs.Mkdir(b.sc, dir, name)
}

func (b sftpFiles) create(ctx context.Context, dir, name string) (string, error) {
	return remotefs.CreateFile(b.sc, dir, name)
}

func (b sftpFiles) removeAll(ctx context.Context, p string) error {
	return remotefs.RemoveAll(ctx, b.sc, p)
}

// adbFileBackend is a device's files, over its sync service.
type adbFileBackend struct{ conn *adb.Conn }

func (b adbFileBackend) rename(ctx context.Context, dir, old, name string) (string, error) {
	from, to := path.Join(dir, old), path.Join(dir, name)
	if to == from {
		return to, nil
	}
	if _, err := b.conn.Stat(ctx, to); err == nil {
		return "", &remotefs.ExistsError{Name: name}
	}
	return to, b.conn.Rename(ctx, from, to)
}

func (b adbFileBackend) mkdir(ctx context.Context, dir, name string) (string, error) {
	p := path.Join(dir, name)
	if _, err := b.conn.Stat(ctx, p); err == nil {
		return "", &remotefs.ExistsError{Name: name}
	}
	return p, b.conn.Mkdir(ctx, p)
}

func (b adbFileBackend) create(ctx context.Context, dir, name string) (string, error) {
	p := path.Join(dir, name)
	if _, err := b.conn.Stat(ctx, p); err == nil {
		return "", &remotefs.ExistsError{Name: name}
	}
	out, err := b.conn.CreateFile(ctx, p)
	if err != nil {
		return "", err
	}
	return p, out.Close()
}

func (b adbFileBackend) removeAll(ctx context.Context, p string) error {
	return b.conn.Remove(ctx, p)
}

// fileBackendOf is how the file panel's dialogs act on s's files, or nil
// when s has none of its own.
func (a *App) fileBackendOf(s *Session) fileBackend {
	if s == nil {
		return nil
	}
	if s.sftp != nil {
		return sftpFiles{s.sftp}
	}
	if s.isADB() {
		if conn := a.adbConn(s); conn != nil {
			return adbFileBackend{conn}
		}
	}
	return nil
}
