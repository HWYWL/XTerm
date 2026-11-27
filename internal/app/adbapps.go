package app

// The apps tab of an ADB tab: the apps installed on the device, the APKs
// dropped on it or chosen to install, and what is done with a chosen app
// (adbapps.go, adbinstall.go).

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"finallyshell/internal/icons"
	"finallyshell/internal/remotefs"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// adbApp is an app installed on the device.
type adbApp struct {
	Package string
	Path    string // the base APK
	System  bool
}

// adbAppsState is a session's apps tab.
type adbAppsState struct {
	list      []adbApp
	loaded    bool
	loading   bool
	err       string
	filter    string
	system    bool // the list includes the system's apps
	unzip     bool // the device has unzip, so labels cost little
	table     ui.ListState
	row       int
	inited    bool
	ask       *adbAppAsk        // a question before deleting what it changes
	info      map[string]*appInfo // labels and icons, by package
	asking    map[string]bool     // the apps whose label is being read
	failed    map[string]bool     // the apps whose label could not be read
}

// adbAppAsk asks before an action that cannot be undone.
type adbAppAsk struct {
	open   bool
	motion dialogMotion
	title  string
	body   string
	ok     string
	run    func()
}

// adbAppColumns are the table's: the package, whether the device shipped
// it, and its APK.
func adbAppColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "app", Title: locales.AdbAppsName(), MinWidth: 170},
		{ID: "package", Title: locales.AdbAppsPackage(), MinWidth: 170},
		{ID: "kind", Title: locales.AdbAppsKind(), Width: 80},
	}
}

// shown is the apps matching the filter, in package order.
func (st *adbAppsState) shown() []adbApp {
	query := strings.ToLower(strings.TrimSpace(st.filter))
	out := make([]adbApp, 0, len(st.list))
	for _, a := range st.list {
		if query == "" || strings.Contains(strings.ToLower(a.Package), query) ||
			strings.Contains(strings.ToLower(a.Path), query) {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Package < out[j].Package })
	return out
}

// chosen is the app chosen in the table, when exactly one is.
func (st *adbAppsState) chosen() (adbApp, bool) {
	view := st.shown()
	if st.row < 0 || st.row >= len(view) {
		return adbApp{}, false
	}
	return view[st.row], true
}

// adbAppsTab is the page: install an APK, and manage the apps already on
// the device.
func (a *App) adbAppsTab(c *ui.Context) {
	s := a.connSession()
	if s == nil || !s.isADB() {
		return
	}
	st := &s.adbApps
	if !st.inited {
		st.inited, st.row = true, -1
	}
	hasConn := a.adbConn(s) != nil
	if hasConn && !st.loaded && !st.loading && st.err == "" {
		a.loadADBApps(s)
	}
	ins := &a.adbInstall

	ui.Column(c).Fill().Padding(10, 12).Gap(8).Children(func() {
		zone := ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(6, 10).Radius(8).
			Border(1, theme.Border)
		if zone.FileDragOver() {
			zone.Border(2, theme.Accent).Background(theme.Accent.Alpha(0.06))
		}
		zone.Children(func() {
			ui.Icon(c, icons.Get("upload")).FontSize(20).TextColor(theme.TextMuted)
			ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, locales.AdbInstallDrop()).FontSize(12).SingleLine()
				widgets.Muted(c, locales.AdbSplitHint()).FontSize(10.5).SingleLine()
			})
			if ui.Button(c, locales.AdbInstallPick()).Shrink(0).Clicked() {
				a.pickAPK(s)
			}
		})
		if files := zone.DroppedFiles(); len(files) > 0 {
			a.installAPKs(s, apkFiles(files))
		}
		if !hasConn {
			widgets.Muted(c, locales.AdbInstallNoDevice()).FontSize(12)
			return
		}
		switch {
		case ins.busy:
			frac := 0.0
			if ins.total > 0 {
				frac = float64(ins.sent) / float64(ins.total)
			}
			widgets.UsageBar(c, ins.name, frac, fmt.Sprintf("%.0f%% · %s / %s",
				frac*100, formatBytes(float64(ins.sent)), formatBytes(float64(ins.total))), theme.Accent)
		case ins.msg != "":
			color := theme.Success
			if ins.failed {
				color = theme.Danger
			}
			ui.Text(c, ins.msg).FontSize(11.5).TextColor(color).Wrap()
		}

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.SearchField(c, &st.filter).Placeholder(locales.AdbAppsFilter()).
				Label(locales.AdbAppsFilter()).Grow(1).MinWidth(120)
			sys := st.system
			if ui.Checkbox(c, &sys, locales.AdbAppsSystem()).FontSize(11).Changed() {
				st.system = sys
				st.loaded = false
				a.loadADBApps(s)
			}
			if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Clicked() {
				a.loadADBApps(s)
			}
			if st.loading {
				ui.Spinner(c)
			}
			widgets.Muted(c, fmt.Sprintf(locales.AdbAppsCount(), len(st.list))).FontSize(11)
		})
		switch {
		case st.err != "":
			ui.Text(c, locales.AdbAppsFailed(st.err)).TextColor(theme.Danger).FontSize(11).Wrap()
		case st.loading && !st.loaded:
			widgets.Muted(c, locales.AdbAppsReading()).FontSize(11)
		}

		view := st.shown()
		st.table.Selected = &st.row
		st.table.Key = func(i int) any { return view[i].Package }
		st.table.Label = func(i int) string { return view[i].Package }
		t := ui.Table(c, &st.table, adbAppColumns(), len(view), func(row, col int) {
			app := view[row]
			switch col {
			case 0:
				label, icon := app.Package, (*ui.Bitmap)(nil)
				if info := st.info[app.Package]; info != nil {
					if info.Label != "" {
						label = info.Label
					}
					icon = info.Icon
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if icon != nil {
						ui.Image(c, icon).Size(18, 18).Radius(4).Shrink(0)
					} else {
						ui.Avatar(c, label, nil).Size(20, 20)
					}
					ui.Text(c, label).SingleLine()
				})
			case 1:
				widgets.Muted(c, app.Package).FontSize(10.5).SingleLine()
			case 2:
				if app.System {
					widgets.Muted(c, locales.AdbAppsSystem())
				} else {
					ui.Text(c, locales.AdbAppsUser())
				}
			}
		}).Grow(1)
		if t.Submitted() && st.row >= 0 && st.row < len(view) {
			a.runADBApp(s, "launch", view[st.row].Package)
		}
		if len(view) == 0 && !st.loading && st.err == "" {
			widgets.Muted(c, locales.AdbAppsNone()).FontSize(11)
		}
		t.ContextMenu(func(m *ui.Menu) {
			app, ok := st.chosen()
			if !ok {
				return
			}
			if m.Item(locales.AdbLaunch()).Chosen() {
				a.runADBApp(s, "launch", app.Package)
			}
			if m.Item(locales.AdbForceStop()).Chosen() {
				a.runADBApp(s, "stop", app.Package)
			}
			if m.Item(locales.AdbClearData()).Chosen() {
				a.askADBApp(s, locales.AdbClearAsk(app.Package), "", locales.AdbClearData(), func() {
					a.runADBApp(s, "clear", app.Package)
				})
			}
			if m.Item(locales.AdbExportAPK()).Chosen() {
				a.exportADBApp(s, app)
			}
			m.Separator()
			if m.Item(locales.AdbUninstall()).Chosen() {
				a.askADBApp(s, locales.AdbUninstallAsk(app.Package), "", locales.AdbUninstall(), func() {
					a.runADBApp(s, "uninstall", app.Package)
				})
			}
			if m.Item(locales.AdbCopyPackage()).Chosen() {
				c.WriteClipboard(app.Package)
			}
		})

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			app, one := st.chosen()
			launch := ui.Button(c, locales.AdbLaunch()).Disabled(!one)
			if launch.Clicked() && one {
				a.runADBApp(s, "launch", app.Package)
			}
			uninstall := ui.Button(c, locales.AdbUninstall()).Disabled(!one)
			if uninstall.Clicked() && one {
				a.askADBApp(s, locales.AdbUninstallAsk(app.Package), "", locales.AdbUninstall(), func() {
					a.runADBApp(s, "uninstall", app.Package)
				})
			}
			export := ui.Button(c, locales.AdbExportAPK()).Disabled(!one)
			if export.Clicked() && one {
				a.exportADBApp(s, app)
			}
			ui.Spacer(c)
			if ins.installed > 0 {
				widgets.Muted(c, fmt.Sprintf(locales.AdbInstallCount(), ins.installed)).FontSize(11)
			}
		})
		a.adbAppAskView(c, st)
	})
}

// askADBApp asks a question before an irreversible action.
func (a *App) askADBApp(s *Session, title, body, ok string, run func()) {
	s.adbApps.ask = &adbAppAsk{open: true, title: title, body: body, ok: ok, run: run}
}

// adbAppAskView draws the question a chosen app asks, if there is one.
func (a *App) adbAppAskView(c *ui.Context, st *adbAppsState) {
	ask := st.ask
	if ask == nil {
		return
	}
	open := ask.open
	a.dialogModal(c, &ask.motion, &open, func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, ask.title).FontSize(15).Bold()
			if ask.body != "" {
				ui.Text(c, ask.body).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, locales.Cancel()).Clicked() {
					open = false
				}
				ok := ask.ok
				if ok == "" {
					ok = locales.Delete()
				}
				if ui.PrimaryButton(c, ok).Clicked() {
					open = false
					if run := ask.run; run != nil {
						ask.run = nil
						run()
					}
				}
			})
		})
	})
	if !open {
		st.ask = nil
		return
	}
	ask.open = open
}

// loadADBApps reads the apps installed on the device, the system's too
// when the list shows them.
func (a *App) loadADBApps(s *Session) {
	st := &s.adbApps
	conn := a.adbConn(s)
	if conn == nil || st.loading {
		return
	}
	st.loading, st.err = true, ""
	ctx := adbCtx(s)
	system := st.system
	go func() {
		cmd := "pm list packages -f -3"
		if system {
			cmd = "echo __USER__; pm list packages -f -3; echo __SYSTEM__; pm list packages -f -s"
		}
		out, err := conn.Exec(ctx, cmd)
		unzip := false
		if err == nil {
			if bin, uerr := conn.Run(ctx, "command -v unzip"); uerr == nil && strings.TrimSpace(bin) != "" {
				unzip = true
			}
		}
		var list []adbApp
		if err == nil {
			list = parseADBPackages(out)
		}
		a.post(func() {
			st.loading = false
			if err != nil {
				st.err = err.Error()
			} else {
				st.list, st.loaded = list, true
				st.unzip, st.failed = unzip, nil
				if st.row >= len(st.shown()) {
					st.row = -1
				}
				a.fetchAppInfos(s)
			}
			a.redraw()
		})
	}()
}

// parseADBPackages reads `pm list packages -f`'s
// "package:/path/base.apk=com.app" lines; the ones after __SYSTEM__ are the
// device's own.
func parseADBPackages(out string) []adbApp {
	seen := map[string]bool{}
	var list []adbApp
	system := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "__SYSTEM__":
			system = true
			continue
		case "__USER__":
			system = false
			continue
		}
		line = strings.TrimPrefix(line, "package:")
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		i := strings.LastIndex(line, "=")
		apk, pkg := line[:i], line[i+1:]
		if pkg == "" || seen[pkg] {
			continue
		}
		seen[pkg] = true
		list = append(list, adbApp{Package: pkg, Path: apk, System: system})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Package < list[j].Package })
	return list
}

// runADBApp opens, stops, clears or uninstalls an app, on a goroutine.
func (a *App) runADBApp(s *Session, action, pkg string) {
	conn := a.adbConn(s)
	if conn == nil {
		a.notify(locales.AdbInstallNoDevice())
		return
	}
	ctx := adbCtx(s)
	q := remotefs.ShellQuote(pkg)
	go func() {
		var out string
		var err error
		switch action {
		case "launch":
			out, err = conn.Run(ctx, "monkey -p "+q+" -c android.intent.category.LAUNCHER 1")
			if err == nil && (strings.Contains(out, "No activities found") || strings.Contains(out, "** Error")) {
				err = fmt.Errorf("%s", strings.TrimSpace(out))
			}
		case "stop":
			out, err = conn.Run(ctx, "am force-stop "+q)
		case "clear":
			out, err = conn.Run(ctx, "pm clear "+q)
		case "uninstall":
			out, err = conn.Run(ctx, "pm uninstall --user 0 "+q)
			if err != nil || !strings.Contains(out, "Success") {
				// A package may refuse the user's copy of it: the plain
				// form, and what it says, is the answer then.
				out2, err2 := conn.Run(ctx, "pm uninstall "+q)
				out += out2
				err = err2
			}
		}
		if err == nil && !strings.Contains(out, "Success") && action != "launch" && action != "stop" {
			err = fmt.Errorf("%s", strings.TrimSpace(out))
		}
		a.post(func() {
			if err != nil {
				switch action {
				case "launch":
					a.notify(locales.AdbLaunchFailed(pkg, err.Error()))
				case "stop":
					a.notify(locales.AdbStopFailed(pkg, err.Error()))
				case "clear":
					a.notify(locales.AdbClearFailed(err.Error()))
				case "uninstall":
					a.notify(locales.AdbUninstallFailed(err.Error()))
				}
				return
			}
			switch action {
			case "launch":
				a.notify(locales.AdbLaunched(pkg))
			case "stop":
				a.notify(locales.AdbStopped(pkg))
			case "clear":
				a.notify(locales.AdbCleared(pkg))
			case "uninstall":
				a.notify(locales.AdbUninstalled(pkg))
				a.loadADBApps(s)
			}
		})
	}()
}

// exportADBApp saves an app's base APK where the user chooses.
func (a *App) exportADBApp(s *Session, app adbApp) {
	conn := a.adbConn(s)
	if conn == nil {
		a.notify(locales.AdbInstallNoDevice())
		return
	}
	ctx := adbCtx(s)
	go func() {
		out, err := conn.Run(ctx, "pm path "+remotefs.ShellQuote(app.Package))
		var apk string
		if err == nil {
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "package:") {
					apk = strings.TrimPrefix(line, "package:")
					break
				}
			}
		}
		if apk == "" {
			msg := strings.TrimSpace(out)
			if msg == "" {
				msg = locales.AdbAppsNone()
			}
			a.notify(locales.AdbExportFailed(msg))
			return
		}
		to, err := a.plat.SaveFile(WindowMain, locales.AdbExportAPK(), app.Package+".apk", "", nil)
		if err != nil || to == "" {
			return
		}
		if err := a.pullADBFile(ctx, conn, apk, to); err != nil {
			a.notify(locales.AdbExportFailed(err.Error()))
			return
		}
		a.notify(locales.AdbExported(filepath.Base(to)))
	}()
}
