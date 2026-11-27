package app

// The processes tab of an ADB tab: what the device runs, read over a shell
// channel of its own. The server's same page reads /proc over SSH
// (processes.go); a device's ps prints enough for a list, and kill ends a
// process.

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// adbProcRefresh is how often the tab reads the list while it shows.
const adbProcRefresh = 2 * time.Second

// adbProc is a process of the device, as ps prints it.
type adbProc struct {
	PID   int
	User  string
	State string
	Name  string
	RSS   int64 // bytes, from ps's KiB
}

// adbProcsState is a session's processes tab for a device.
type adbProcsState struct {
	list    []adbProc
	loaded  bool
	loading bool
	err     string
	filter  string
	table   ui.ListState
	sort    ui.SortOrder
	row     int
	pid     int // the process chosen, which a refresh keeps
	inited  bool
	started bool
	seen    bool // drawn since the last stop of the unseen polls
	wake    chan struct{}
	stop    chan struct{}
	ask     *adbKillAsk
}

// adbKillAsk asks before ending a process.
type adbKillAsk struct {
	open   bool
	motion dialogMotion
	pid    int
	name   string
	force  bool
}

// adbProcColumns are the table's columns; their IDs are the sort's.
func adbProcColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "pid", Title: locales.ColPID(), Width: 72, Align: ui.End, Sortable: true},
		{ID: "user", Title: locales.ColProcUser(), Width: 96, Sortable: true},
		{ID: "mem", Title: locales.ColProcMemory(), Width: 88, Align: ui.End, Sortable: true},
		{ID: "state", Title: locales.ColNetState(), Width: 64, Sortable: true},
		{ID: "name", Title: locales.ColProcName(), MinWidth: 180, Sortable: true},
	}
}

// shown is the processes matching the filter, in the order of the sort.
func (st *adbProcsState) shown() []adbProc {
	query := strings.ToLower(strings.TrimSpace(st.filter))
	out := make([]adbProc, 0, len(st.list))
	for _, p := range st.list {
		if query == "" || strings.Contains(strings.ToLower(p.Name), query) ||
			strings.Contains(strings.ToLower(p.User), query) ||
			strconv.Itoa(p.PID) == query {
			out = append(out, p)
		}
	}
	by, desc := st.sort.Column, st.sort.Descending
	slices.SortStableFunc(out, func(x, y adbProc) int {
		c := compareADBProcs(by, x, y)
		if desc {
			c = -c
		}
		if c == 0 {
			return cmp.Compare(x.PID, y.PID)
		}
		return c
	})
	return out
}

// compareADBProcs orders two processes by a column of the table.
func compareADBProcs(by string, x, y adbProc) int {
	switch by {
	case "pid":
		return cmp.Compare(x.PID, y.PID)
	case "user":
		return strings.Compare(x.User, y.User)
	case "mem":
		return cmp.Compare(x.RSS, y.RSS)
	case "state":
		return strings.Compare(x.State, y.State)
	}
	return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
}

// adbProcessesTab is the page: the search, the count, and the list.
func (a *App) adbProcessesTab(c *ui.Context) {
	s := a.connSession()
	if s == nil || !s.isADB() {
		return
	}
	st := &s.adbProcs
	if !st.inited {
		st.inited, st.row = true, -1
		st.sort = ui.SortOrder{Column: "mem", Descending: true}
	}
	if a.adbConn(s) == nil {
		widgets.Muted(c, locales.AdbInstallNoDevice()).FontSize(12)
		return
	}
	st.seen = true
	a.pollADBProcs(s)
	view := st.shown()
	// The process chosen stays chosen across a read, when it is still there.
	st.row = -1
	if st.pid != 0 {
		st.row = slices.IndexFunc(view, func(p adbProc) bool { return p.PID == st.pid })
	}

	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			ui.SearchField(c, &st.filter).Placeholder(locales.ProcFilter()).
				Label(locales.ProcFilter()).Width(300).Shrink(1).MinWidth(120)
			if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Clicked() {
				select {
				case st.wake <- struct{}{}:
				default:
				}
			}
			switch {
			case st.loading && !st.loaded:
				ui.Spinner(c)
				widgets.Muted(c, locales.ProcReading()).FontSize(11)
			case st.err != "":
				ui.Text(c, locales.AdbProcFailed(st.err)).TextColor(theme.Danger).
					FontSize(11).Shrink(1).MinWidth(0).Wrap()
			default:
				n, shown := len(st.list), len(view)
				text := locales.ProcCount(n)
				if shown != n {
					text = locales.ProcCountFiltered(shown, n)
				}
				widgets.Muted(c, text).FontSize(11)
			}
			ui.Spacer(c)
		})
		st.table.Selected = &st.row
		st.table.Sort = &st.sort
		st.table.KeepOffset = true
		st.table.Key = func(i int) any { return view[i].PID }
		st.table.Label = func(i int) string { return view[i].Name }
		t := ui.Table(c, &st.table, adbProcColumns(), len(view), func(row, col int) {
			p := view[row]
			switch col {
			case 0:
				ui.Text(c, strconv.Itoa(p.PID))
			case 1:
				ui.Text(c, p.User).SingleLine()
			case 2:
				ui.Text(c, formatBytes(float64(p.RSS)))
			case 3:
				ui.Text(c, p.State)
			case 4:
				ui.Text(c, p.Name).SingleLine()
			}
		}).Grow(1)
		if st.row >= 0 && st.row < len(view) {
			st.pid = view[st.row].PID
			p := view[st.row]
			t.ContextMenu(func(m *ui.Menu) {
				if p.PID > 1 {
					if m.Item(locales.AdbProcKill()).Chosen() {
						a.askADBKill(s, p, false)
					}
					if m.Item(locales.AdbProcKillForce()).Chosen() {
						a.askADBKill(s, p, true)
					}
					m.Separator()
				}
				if m.Item(locales.AdbProcCopyPID()).Chosen() {
					c.WriteClipboard(strconv.Itoa(p.PID))
				}
			})
		} else {
			st.pid = 0
		}
		if len(view) == 0 && !st.loading && st.err == "" {
			widgets.Muted(c, locales.AdbProcNone()).Padding(12, 14)
		}
		ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			p, one := adbProc{}, false
			if st.row >= 0 && st.row < len(view) {
				p, one = view[st.row], true
			}
			if ui.Button(c, locales.AdbProcKill()).Disabled(!one || p.PID <= 1).Clicked() {
				a.askADBKill(s, p, false)
			}
			if ui.Button(c, locales.AdbProcKillForce()).Disabled(!one || p.PID <= 1).Clicked() {
				a.askADBKill(s, p, true)
			}
		})
		a.adbKillAskView(c, s, st)
	})
}

// pollADBProcs reads the device's processes every couple of seconds, on a
// goroutine of its own, while the connection is up.
func (a *App) pollADBProcs(s *Session) {
	st := &s.adbProcs
	if st.started {
		return
	}
	if a.adbConn(s) == nil {
		return
	}
	if st.wake == nil {
		st.wake = make(chan struct{}, 1)
	}
	st.stop = make(chan struct{})
	stop := st.stop
	st.started, st.loading = true, true
	ctx := adbCtx(s)
	go func() {
		defer a.post(func() {
			st.started, st.loading = false, false
			a.redraw()
		})
		for {
			conn := a.adbConn(s)
			if conn == nil || ctx.Err() != nil {
				return
			}
			out, err := conn.Exec(ctx, "ps -A -o PID,USER,RSS,VSZ,S,NAME")
			var list []adbProc
			if err == nil {
				list, err = parseADBProcs(out)
			}
			a.post(func() {
				st.loading = false
				if err != nil {
					st.err = err.Error()
				} else {
					st.list, st.loaded, st.err = list, true, ""
				}
				a.redraw()
			})
			select {
			case <-time.After(adbProcRefresh):
			case <-st.wake:
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// parseADBProcs reads `ps -A -o PID,USER,RSS,VSZ,S,NAME`: the header, then
// a line for each process, its name last and possibly with spaces.
func parseADBProcs(out string) ([]adbProc, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || !strings.Contains(strings.ToUpper(lines[0]), "PID") {
		return nil, fmt.Errorf("%s", strings.TrimSpace(out))
	}
	var list []adbProc
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		rss, _ := strconv.ParseInt(f[2], 10, 64)
		list = append(list, adbProc{
			PID: pid, User: f[1], RSS: rss * 1024, State: f[4],
			Name: strings.Join(f[5:], " "),
		})
	}
	return list, nil
}

// halt stops a poll that has not been drawn since the last frame.
func (st *adbProcsState) halt() {
	if st.stop != nil {
		close(st.stop)
		st.stop = nil
	}
}

// askADBKill asks before ending a process.
func (a *App) askADBKill(s *Session, p adbProc, force bool) {
	s.adbProcs.ask = &adbKillAsk{open: true, pid: p.PID, name: p.Name, force: force}
}

// adbKillAskView draws the question before a process is ended.
func (a *App) adbKillAskView(c *ui.Context, s *Session, st *adbProcsState) {
	ask := st.ask
	if ask == nil {
		return
	}
	open := ask.open
	label := locales.AdbProcKill()
	if ask.force {
		label = locales.AdbProcKillForce()
	}
	a.dialogModal(c, &ask.motion, &open, func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, label).FontSize(15).Bold()
			ui.Text(c, locales.AdbProcKillAsk(ask.name, strconv.Itoa(ask.pid))).Wrap()
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, locales.Cancel()).Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, label).Clicked() {
					open = false
					st.ask = nil
					a.killADBProcess(s, st, ask)
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

// killADBProcess sends SIGTERM or SIGKILL to a process of the device.
func (a *App) killADBProcess(s *Session, st *adbProcsState, ask *adbKillAsk) {
	conn := a.adbConn(s)
	if conn == nil {
		return
	}
	ctx := adbCtx(s)
	sig := "-TERM"
	if ask.force {
		sig = "-KILL"
	}
	go func() {
		_, err := conn.Run(ctx, "kill "+sig+" "+strconv.Itoa(ask.pid))
		a.post(func() {
			if err != nil {
				a.notify(locales.AdbProcKillFailed(err.Error()))
			} else {
				a.notify(locales.AdbProcKilled(ask.name))
			}
			select {
			case st.wake <- struct{}{}:
			default:
			}
			a.redraw()
		})
	}()
}
