package app

// The network tab of an ADB tab: the device's sockets, read with ss or
// netstat over a shell channel of its own.

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// adbNetRefresh is how often the tab reads the sockets while it shows.
const adbNetRefresh = 3 * time.Second

// adbNetScript prints the device's sockets with whichever tool it has:
// ss where it is, netstat otherwise, and a word when it has neither.
const adbNetScript = `if command -v ss >/dev/null 2>&1; then ss -tunap; elif command -v netstat >/dev/null 2>&1; then netstat -tunap; else echo __XTERM_NO_NET__; fi`

// errADBNetTools is a device with neither ss nor netstat.
var errADBNetTools = errors.New("no ss or netstat")

// adbSock is a socket of the device.
type adbSock struct {
	Proto   string
	Local   string
	Remote  string
	State   string
	PID     int
	Program string
}

// key identifies a socket across a refresh.
func (s adbSock) key() string { return s.Proto + "|" + s.Local + "|" + s.Remote + "|" + strconv.Itoa(s.PID) }

// adbNetState is a session's network tab for a device.
type adbNetState struct {
	list       []adbSock
	loaded     bool
	loading    bool
	err        string
	filter     string
	listenOnly bool
	table      ui.ListState
	sort       ui.SortOrder
	row        int
	key        string
	inited     bool
	started    bool
	seen       bool // drawn since the last stop of the unseen polls
	wake       chan struct{}
	stop       chan struct{}
}

// adbSockColumns are the table's columns; their IDs are the sort's.
func adbSockColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "proto", Title: locales.ColNetProto(), Width: 70, Sortable: true},
		{ID: "local", Title: locales.AdbNetLocal(), MinWidth: 160, Sortable: true},
		{ID: "remote", Title: locales.AdbNetRemote(), MinWidth: 160, Sortable: true},
		{ID: "state", Title: locales.ColNetState(), Width: 84, Sortable: true},
		{ID: "pid", Title: locales.ColPID(), Width: 64, Align: ui.End, Sortable: true},
		{ID: "program", Title: locales.AdbNetProgram(), MinWidth: 110, Sortable: true},
	}
}

// shown is the sockets matching the filter, in the order of the sort.
func (st *adbNetState) shown() []adbSock {
	query := strings.ToLower(strings.TrimSpace(st.filter))
	out := make([]adbSock, 0, len(st.list))
	for _, s := range st.list {
		if st.listenOnly && s.State != "LISTEN" {
			continue
		}
		if query != "" &&
			!strings.Contains(strings.ToLower(s.Local), query) &&
			!strings.Contains(strings.ToLower(s.Remote), query) &&
			!strings.Contains(strings.ToLower(s.State), query) &&
			!strings.Contains(strings.ToLower(s.Program), query) &&
			!strings.Contains(strings.ToLower(s.Proto), query) &&
			!strings.Contains(strconv.Itoa(s.PID), query) {
			continue
		}
		out = append(out, s)
	}
	by, desc := st.sort.Column, st.sort.Descending
	slices.SortStableFunc(out, func(x, y adbSock) int {
		c := compareADBSocks(by, x, y)
		if desc {
			c = -c
		}
		if c == 0 {
			return strings.Compare(x.key(), y.key())
		}
		return c
	})
	return out
}

// compareADBSocks orders two sockets by a column of the table.
func compareADBSocks(by string, x, y adbSock) int {
	switch by {
	case "proto":
		return strings.Compare(x.Proto, y.Proto)
	case "local":
		return strings.Compare(x.Local, y.Local)
	case "remote":
		return strings.Compare(x.Remote, y.Remote)
	case "state":
		return strings.Compare(x.State, y.State)
	case "pid":
		return cmp.Compare(x.PID, y.PID)
	}
	return strings.Compare(strings.ToLower(x.Program), strings.ToLower(y.Program))
}

// adbNetworkTab is the page: the search, the count, and the sockets.
func (a *App) adbNetworkTab(c *ui.Context) {
	s := a.connSession()
	if s == nil || !s.isADB() {
		return
	}
	st := &s.adbNet
	if !st.inited {
		st.inited, st.row = true, -1
	}
	if a.adbConn(s) == nil {
		widgets.Muted(c, locales.AdbInstallNoDevice()).FontSize(12)
		return
	}
	st.seen = true
	a.pollADBNet(s)
	view := st.shown()
	st.row = -1
	if st.key != "" {
		st.row = slices.IndexFunc(view, func(x adbSock) bool { return x.key() == st.key })
	}

	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			ui.SearchField(c, &st.filter).Placeholder(locales.NetFilter()).
				Label(locales.NetFilter()).Width(300).Shrink(1).MinWidth(120)
			listen := st.listenOnly
			if ui.Checkbox(c, &listen, locales.AdbNetListenOnly()).FontSize(11).Changed() {
				st.listenOnly = listen
			}
			if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Clicked() {
				select {
				case st.wake <- struct{}{}:
				default:
				}
			}
			switch {
			case st.loading && !st.loaded:
				ui.Spinner(c)
				widgets.Muted(c, locales.AdbNetReading()).FontSize(11)
			case st.err != "":
				ui.Text(c, st.err).TextColor(theme.Danger).FontSize(11).
					Shrink(1).MinWidth(0).Wrap()
			default:
				widgets.Muted(c, fmt.Sprintf(locales.AdbNetCount(), len(view))).FontSize(11)
			}
			ui.Spacer(c)
		})
		st.table.Selected = &st.row
		st.table.Sort = &st.sort
		st.table.KeepOffset = true
		st.table.Key = func(i int) any { return view[i].key() }
		st.table.Label = func(i int) string { return view[i].Local }
		t := ui.Table(c, &st.table, adbSockColumns(), len(view), func(row, col int) {
			x := view[row]
			switch col {
			case 0:
				ui.Text(c, x.Proto)
			case 1:
				ui.Text(c, x.Local).SingleLine()
			case 2:
				ui.Text(c, x.Remote).SingleLine()
			case 3:
				ui.Text(c, x.State)
			case 4:
				if x.PID != 0 {
					ui.Text(c, strconv.Itoa(x.PID))
				}
			case 5:
				ui.Text(c, x.Program).SingleLine()
			}
		}).Grow(1)
		if st.row >= 0 && st.row < len(view) {
			st.key = view[st.row].key()
			x := view[st.row]
			t.ContextMenu(func(m *ui.Menu) {
				if m.Item(locales.AdbNetCopyLocal()).Chosen() {
					c.WriteClipboard(x.Local)
				}
				if x.Remote != "" && m.Item(locales.AdbNetCopyRemote()).Chosen() {
					c.WriteClipboard(x.Remote)
				}
			})
		} else {
			st.key = ""
		}
		if len(view) == 0 && !st.loading && st.err == "" {
			widgets.Muted(c, locales.AdbNetNone()).Padding(12, 14)
		}
	})
}

// halt stops a poll that has not been drawn since the last frame.
func (st *adbNetState) halt() {
	if st.stop != nil {
		close(st.stop)
		st.stop = nil
	}
}

// pollADBNet reads the device's sockets every few seconds, on a goroutine
// of its own, while the connection is up.
func (a *App) pollADBNet(s *Session) {
	st := &s.adbNet
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
			out, err := conn.Exec(ctx, adbNetScript)
			var list []adbSock
			if err == nil {
				list, err = parseADBSockets(out)
			}
			a.post(func() {
				st.loading = false
				switch {
				case errors.Is(err, errADBNetTools):
					st.err = locales.AdbNetNoTools()
				case err != nil:
					st.err = locales.AdbNetFailed(err.Error())
				default:
					st.list, st.loaded, st.err = list, true, ""
				}
				a.redraw()
			})
			select {
			case <-time.After(adbNetRefresh):
			case <-st.wake:
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

var (
	adbSockPID     = regexp.MustCompile(`pid=(\d+)`)
	adbSockProgram = regexp.MustCompile(`\("([^"]+)"`)
)

// parseADBSockets reads `ss -tunap` or `netstat -tunap`: the proto, the
// local and remote addresses, the state, and the process that holds it
// where the output says.
func parseADBSockets(out string) ([]adbSock, error) {
	if strings.Contains(out, "__XTERM_NO_NET__") {
		return nil, errADBNetTools
	}
	var list []adbSock
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		if isADBProto(f[0]) && len(f) >= 6 {
			// netstat: Proto Recv-Q Send-Q Local Foreign State [PID/Program]
			if _, err := strconv.Atoi(f[1]); err == nil {
				x := adbSock{Proto: f[0], Local: f[3], Remote: f[4], State: f[5]}
				if len(f) > 6 {
					if pid, prog, ok := parseNetstatProc(f[6]); ok {
						x.PID, x.Program = pid, prog
					}
				}
				list = append(list, x)
				continue
			}
		}
		// ss: [Netid] Proto State Recv-Q Send-Q Local Peer Process
		i := slices.IndexFunc(f, isADBProto)
		if i < 0 || i+5 >= len(f) {
			continue
		}
		x := adbSock{Proto: f[i], State: f[i+1], Local: f[i+4], Remote: f[i+5]}
		rest := strings.Join(f[i+6:], " ")
		if m := adbSockProgram.FindStringSubmatch(rest); m != nil {
			x.Program = m[1]
		}
		if m := adbSockPID.FindStringSubmatch(rest); m != nil {
			x.PID, _ = strconv.Atoi(m[1])
		}
		list = append(list, x)
	}
	return list, nil
}

// isADBProto tells whether f is one of the protocols ss and netstat print.
func isADBProto(f string) bool {
	switch f {
	case "tcp", "tcp6", "udp", "udp6", "raw", "raw6":
		return true
	}
	return false
}

// parseNetstatProc reads netstat's "1234/program", or its "-".
func parseNetstatProc(f string) (int, string, bool) {
	if f == "-" {
		return 0, "", false
	}
	if i := strings.IndexByte(f, '/'); i >= 0 {
		if pid, err := strconv.Atoi(f[:i]); err == nil {
			return pid, f[i+1:], true
		}
		return 0, f, true
	}
	if pid, err := strconv.Atoi(f); err == nil {
		return pid, "", true
	}
	return 0, f, true
}

