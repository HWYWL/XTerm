package app

// Android's platform-tools, which a device on a cable is reached through on
// Windows and macOS (internal/adb). The app fetches them into its own
// folder, so that nobody has to install anything by hand.

import (
	"context"
	"errors"
	"fmt"

	"finallyshell/internal/adb"
	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// adbTools is the fetch of platform-tools, as the pages show it: how far
// it has come, and how it went.
type adbTools struct {
	busy  bool
	stage adb.Stage
	done  int64
	total int64
	err   string
	path  string // the adb it left, once it is done
}

// installTools is adb.InstallTools, which a test replaces.
var installTools = adb.InstallTools

// installADBTools fetches platform-tools into the settings folder, so that
// a device on a cable works without the user installing anything.
func (a *App) installADBTools() {
	st := &a.adbtools
	if st.busy {
		return
	}
	st.busy, st.err, st.path = true, "", ""
	st.stage, st.done, st.total = adb.StageDownload, 0, 0
	a.redraw()
	go func() {
		path, err := installTools(context.Background(), config.Dir(), func(p adb.Progress) {
			a.post(func() {
				st.stage, st.done, st.total = p.Stage, p.Done, p.Total
				a.redraw()
			})
		})
		a.post(func() {
			st.busy = false
			if err != nil {
				st.err = err.Error()
			} else {
				st.path = path
			}
			a.redraw()
		})
	}()
}

// adbToolsRow is the offer of platform-tools: a button while they are not
// there, how far their download has come, and what went wrong.
func (a *App) adbToolsRow(c *ui.Context) {
	st := &a.adbtools
	switch {
	case st.busy:
		text := locales.AdbToolsUnpacking()
		frac := 1.0
		if st.stage == adb.StageDownload {
			frac = 0
			if st.total > 0 {
				frac = float64(st.done) / float64(st.total)
			}
			text = fmt.Sprintf("%.0f%% · %s / %s", frac*100,
				formatBytes(float64(st.done)), formatBytes(float64(st.total)))
		}
		widgets.UsageBar(c, "", frac, text, theme.Accent)
	case st.err != "":
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.AdbToolsFailed(st.err)).FontSize(11).
				TextColor(theme.Danger).Grow(1).MinWidth(0).Wrap()
			if ui.Button(c, locales.AdbToolsRetry()).Shrink(0).Clicked() {
				a.installADBTools()
			}
		})
	case st.path != "":
		ui.Row(c).Children(func() {
			widgets.Muted(c, locales.AdbToolsReady()).FontSize(11).Wrap()
		})
	default:
		ui.Row(c).Children(func() {
			if ui.Button(c, locales.AdbToolsDownload()).
				Tooltip(locales.AdbToolsHint()).Clicked() {
				a.installADBTools()
			}
		})
	}
}

// adbNeedsTools says the session failed because the ADB server of this
// computer is not there, which platform-tools brings.
func (s *Session) adbNeedsTools() bool {
	return s != nil && s.isADB() && s.status != statusConnecting && errors.Is(s.err, adb.ErrNoServer)
}
