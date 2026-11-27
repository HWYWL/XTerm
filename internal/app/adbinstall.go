package app

// Installing an app on an ADB tab: an APK or a base with its split APKs is
// copied to the device and installed. The apps tab shows the install, and
// the apps already there (adbapps.go).

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"finallyshell/internal/adb"
	"finallyshell/locales"
)

// adbInstall is the install the apps tab shows: how far it has come, and
// how it went.
type adbInstall struct {
	busy      bool
	name      string
	sent      int64
	total     int64
	msg       string
	failed    bool
	installed int
}

// installAPK is (*adb.Conn).Install, which a test replaces.
var installAPK = func(c *adb.Conn, ctx context.Context, apk string, progress func(sent, total int64)) (string, error) {
	return c.Install(ctx, apk, progress)
}

// installMultipleAPK is (*adb.Conn).InstallMultiple, for a base APK with
// its splits.
var installMultipleAPK = func(c *adb.Conn, ctx context.Context, apks []string, progress func(sent, total int64)) (string, error) {
	return c.InstallMultiple(ctx, apks, progress)
}

// pickAPK asks for APKs and installs them.
func (a *App) pickAPK(s *Session) {
	go func() {
		files, err := a.plat.OpenFiles(locales.AdbInstallPick(), false)
		if err != nil || len(files) == 0 {
			return
		}
		a.post(func() { a.installAPKs(s, apkFiles(files)) })
	}()
}

// apkFiles are the APKs and APK bundles of a drop or a choice: the rest
// are left alone.
func apkFiles(files []string) []string {
	var out []string
	for _, f := range files {
		switch strings.ToLower(path.Ext(f)) {
		case ".apk", ".apks", ".xapk":
			out = append(out, f)
		}
	}
	return out
}

// expandAPKs are the APK files to install: those given, and the ones in a
// bundle (.apks or .xapk, which are zips of a base APK and its splits). It
// returns the folder of the ones it unpacked, to remove after.
func expandAPKs(paths []string) ([]string, func(), error) {
	var (
		apks   []string
		root   string
		bundle int
	)
	cleanup := func() {
		if root != "" {
			os.RemoveAll(root)
		}
	}
	for _, p := range paths {
		if strings.EqualFold(path.Ext(p), ".apk") {
			apks = append(apks, p)
			continue
		}
		zr, err := zip.OpenReader(p)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		if root == "" {
			if root, err = os.MkdirTemp("", "xterm-apks-"); err != nil {
				zr.Close()
				return nil, func() {}, err
			}
		}
		bundle++
		dir := filepath.Join(root, strconv.Itoa(bundle))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			zr.Close()
			cleanup()
			return nil, func() {}, err
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || !strings.EqualFold(path.Ext(f.Name), ".apk") {
				continue
			}
			in, err := f.Open()
			if err != nil {
				zr.Close()
				cleanup()
				return nil, func() {}, err
			}
			out, err := os.Create(filepath.Join(dir, filepath.Base(f.Name)))
			if err != nil {
				in.Close()
				zr.Close()
				cleanup()
				return nil, func() {}, err
			}
			_, err = io.Copy(out, in)
			in.Close()
			out.Close()
			if err != nil {
				zr.Close()
				cleanup()
				return nil, func() {}, err
			}
			apks = append(apks, filepath.Join(dir, filepath.Base(f.Name)))
		}
		zr.Close()
	}
	if len(apks) == 0 {
		cleanup()
		return nil, func() {}, fmt.Errorf("no APK in the selection")
	}
	return apks, cleanup, nil
}

// installAPKs installs apks on a goroutine, showing their progress in the
// tab: one APK on its own, or the base and its splits together.
func (a *App) installAPKs(s *Session, apks []string) {
	st := &a.adbInstall
	if st.busy || len(apks) == 0 {
		return
	}
	conn := a.adbConn(s)
	if conn == nil {
		st.msg, st.failed = locales.AdbInstallNoDevice(), true
		a.redraw()
		return
	}
	st.busy, st.failed, st.msg, st.installed = true, false, "", 0
	st.sent, st.total = 0, 0
	a.redraw()
	ctx := adbCtx(s)
	go func() {
		apks, cleanup, err := expandAPKs(apks)
		defer cleanup()
		if err != nil {
			a.post(func() {
				st.busy = false
				st.msg, st.failed = locales.AdbInstallFailed(err.Error()), true
				a.redraw()
			})
			return
		}
		var lastErr string
		done := 0
		report := func(sent, total int64) {
			a.post(func() {
				st.sent, st.total = sent, total
				a.redraw()
			})
		}
		if len(apks) > 1 {
			st.name = fmt.Sprintf(locales.AdbInstallMany(), len(apks))
			a.post(func() { a.redraw() })
			if _, err := installMultipleAPK(conn, ctx, apks, report); err != nil {
				lastErr = err.Error()
			} else {
				done = 1
			}
		} else {
			st.name = path.Base(apks[0])
			a.post(func() { a.redraw() })
			if _, err := installAPK(conn, ctx, apks[0], report); err != nil {
				lastErr = err.Error()
			} else {
				done = 1
			}
		}
		a.post(func() {
			st.busy, st.installed = false, done
			if lastErr != "" {
				st.msg, st.failed = locales.AdbInstallFailed(lastErr), true
			} else {
				st.msg, st.failed = locales.AdbInstallOK(), false
				a.loadADBApps(s)
			}
			a.redraw()
		})
	}()
}
