package app

// The apps tab of an ADB tab's bottom panel: an APK dropped on it, or
// chosen with a picker, is copied to the device and installed.

import (
	"context"
	"fmt"
	"path"
	"strings"

	"finallyshell/internal/adb"
	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
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

// adbAppsTab is the bottom panel's page for installing an app: a drop
// zone, and the last install's progress and result.
func (a *App) adbAppsTab(c *ui.Context) {
	s := a.connSession()
	st := &a.adbInstall
	ui.Column(c).Fill().Padding(12, 14).Gap(10).Children(func() {
		widgets.Muted(c, locales.AdbInstallHint()).FontSize(12).Wrap()
		zone := ui.Column(c).Grow(1).MinHeight(60).Center().Gap(8).Radius(8).
			Border(2, theme.Border).Padding(18)
		if zone.FileDragOver() {
			zone.Border(2, theme.Accent).Background(theme.Accent.Alpha(0.06))
		}
		zone.Children(func() {
			ui.Icon(c, icons.Get("upload")).FontSize(26).TextColor(theme.TextMuted)
			ui.Text(c, locales.AdbInstallDrop()).FontSize(12).TextColor(theme.TextMuted)
			if ui.Button(c, locales.AdbInstallPick()).Clicked() {
				a.pickAPK(s)
			}
		})
		if files := zone.DroppedFiles(); len(files) > 0 {
			a.installAPKs(s, apkFiles(files))
		}
		switch {
		case st.busy:
			frac := 0.0
			if st.total > 0 {
				frac = float64(st.sent) / float64(st.total)
			}
			widgets.UsageBar(c, st.name, frac, fmt.Sprintf("%.0f%% · %s / %s",
				frac*100, formatBytes(float64(st.sent)), formatBytes(float64(st.total))), theme.Accent)
		case st.msg != "":
			color := theme.Success
			if st.failed {
				color = theme.Danger
			}
			ui.Text(c, st.msg).FontSize(11.5).TextColor(color).Wrap()
		}
		if st.installed > 0 {
			widgets.Muted(c, fmt.Sprintf(locales.AdbInstallCount(), st.installed)).FontSize(11)
		}
	})
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

// apkFiles are the APKs of a drop or a choice: the rest are left alone.
func apkFiles(files []string) []string {
	var out []string
	for _, f := range files {
		if strings.EqualFold(path.Ext(f), ".apk") {
			out = append(out, f)
		}
	}
	return out
}

// installAPKs installs apks one after the other, on a goroutine, showing
// each one's progress in the panel.
func (a *App) installAPKs(s *Session, apks []string) {
	st := &a.adbInstall
	if st.busy || len(apks) == 0 {
		return
	}
	m := a.adbMeter(s)
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		st.msg, st.failed = locales.AdbInstallNoDevice(), true
		a.redraw()
		return
	}
	st.busy, st.failed, st.msg, st.installed = true, false, "", 0
	a.redraw()
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		var lastErr string
		done := 0
		for _, apk := range apks {
			st.name = path.Base(apk)
			st.sent, st.total = 0, 0
			a.post(func() { a.redraw() })
			_, err := installAPK(conn, ctx, apk, func(sent, total int64) {
				a.post(func() {
					st.sent, st.total = sent, total
					a.redraw()
				})
			})
			if err != nil {
				lastErr = err.Error()
				continue
			}
			done++
		}
		a.post(func() {
			st.busy, st.installed = false, done
			if lastErr != "" {
				st.msg, st.failed = locales.AdbInstallFailed(lastErr), true
			} else {
				st.msg, st.failed = locales.AdbInstallOK(), false
			}
			a.redraw()
		})
	}()
}
