package app

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"finallyshell/internal/icons"
	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// monitorPanel shows the active session's server, as FinalShell's left
// column does: the skeleton for a new tab. Its context menu chooses the
// parts that show (monitorparts.go). A local terminal has no server: a
// line says what it is.
func (a *App) monitorPanel(c *ui.Context) {
	s := a.connSession()
	if s != nil && s.isNewTab() {
		s = nil
	}
	ui.Scroll(c).Fill().ContextMenu(a.monitorMenu).Children(func() {
		if s.isLocal() {
			a.localMonitor(c)
			return
		}
		a.serverView(c, s)
	})
}

// serverView is a server as the side panel shows it: its IP, the System
// info button, and its stats, without the parts the user hid. Without a
// session, or before the first stats, it is the skeleton FinalShell
// shows: dashes and empty bars.
func (a *App) serverView(c *ui.Context, s *Session) {
	ui.Column(c).FillWidth().Padding(10, 10, 8, 10).Gap(6).Children(func() {
		// The IP, with all the width it can have, and the copy of it tucked
		// against the System info button at the end of the line, so the IP
		// keeps the room before them.
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ip := "-"
			if s != nil {
				ip = s.ip
				if ip == "" {
					ip = s.Host.Addr
				}
			}
			ui.Text(c, "IP "+ip).SingleLine().MinWidth(0)
			ui.Spacer(c)
			ui.Row(c).Gap(0).AlignItems(ui.Center).Shrink(0).Children(func() {
				if s != nil {
					// The copy, a button the size of the text.
					cp := ui.Box(c).Size(16, 16).Center().Radius(4).Shrink(0).
						Label(locales.Copy()).Tooltip(locales.Copy()).Cursor(ui.CursorPointer)
					if cp.Hovered() {
						cp.Background(theme.Text.Alpha(0.07))
					}
					cp.Children(func() {
						ui.Icon(c, icons.Get("copy")).FontSize(12).TextColor(theme.Accent)
					})
					if cp.Clicked() {
						a.copyToClipboard(c, ip)
					}
				}
				a.sysInfoButton(c, s)
			})
		})
		var st *monitor.Stats
		switch {
		case s == nil:
		case s.status != statusConnected:
			widgets.Muted(c, s.statusText())
		case !execAllowed(s.Host):
			widgets.Muted(c, locales.MonitorOff())
		case !s.stats.OK:
			widgets.Muted(c, locales.ReadingServerInfo())
		default:
			st = &s.stats
		}
		var today func(monitor.NetView) (uint64, uint64)
		if s != nil && s.Host != nil {
			today = a.netToday(s.Host.ID)
		}
		serverStats(c, st, a.monitorHidden(), a.netPicker(s), &a.localNet, today)
	})
}

// copyToClipboard puts text in the clipboard, and says so in a toast at the
// bottom of the window, which fades out two seconds later.
func (a *App) copyToClipboard(c *ui.Context, text string) {
	c.WriteClipboard(text)
	c.AddToast(ui.Toast{ID: "copied", Title: locales.Copied(), Timeout: 2 * time.Second})
}

// serverStats shows st, or the skeleton for nil, without the parts in
// hidden, or the hint to the panel's menu when every part is; net is the
// choice of network interfaces, and today the traffic of the day so far,
// when a session is there to keep it for.
func serverStats(c *ui.Context, st *monitor.Stats, hidden hiddenParts, net netPicker, local *localTraffic, today func(monitor.NetView) (uint64, uint64)) {
	if hidden.none() {
		monitorHint(c)
		return
	}
	if st == nil {
		if hidden.shows(partUptime) {
			ui.Text(c, locales.UptimeUnknown())
			ui.Text(c, locales.LoadAverage("-"))
		}
		if hidden.shows(partCPU) {
			section(c, locales.Resources(), nil)
			widgets.UsageBar(c, locales.Cpu(), 0, "0%", colors.cpu)
			widgets.UsageBar(c, locales.Memory(), 0, "0%", colors.mem)
			widgets.UsageBar(c, locales.Swap(), 0, "0%", colors.swap)
			widgets.UsageBar(c, locales.SysDisks(), 0, "0%", colors.disk)
			donutRow(c, nil)
			statCards(c, nil)
		}
		if hidden.shows(partNetwork) {
			netCard(c, monitor.NetView{}, "", "", nil, 0, 0)
		}
		if hidden.shows(partLocalNet) {
			localNetGraph(c, nil)
		}
		if hidden.shows(partLatency) {
			latencyCard(c, 0, nil)
		}
		if hidden.shows(partDisks) {
			section(c, locales.MonitorPartDisks(), nil)
			diskTable(c, nil)
		}
		return
	}
	if hidden.shows(partUptime) {
		ui.Text(c, locales.UptimeDays(int(st.Uptime.Hours()/24)))
		ui.Text(c, locales.LoadAverage(fmt.Sprintf("%.2f, %.2f, %.2f", st.Load[0], st.Load[1], st.Load[2])))
	}
	if hidden.shows(partCPU) {
		section(c, locales.Resources(), nil)
		widgets.UsageBar(c, locales.Cpu(), st.CPU/100, fmt.Sprintf("%.0f%%", st.CPU), colors.cpu)
		widgets.UsageBar(c, locales.Memory(), ratio(st.MemUsed, st.MemTotal), usage(st.MemUsed, st.MemTotal), colors.mem)
		widgets.UsageBar(c, locales.Swap(), ratio(st.SwapUsed, st.SwapTotal), usage(st.SwapUsed, st.SwapTotal), colors.swap)
		diskUsed, diskTotal := diskUsage(st.Disks)
		widgets.UsageBar(c, locales.SysDisks(), ratio(diskUsed, diskTotal), usage(diskUsed, diskTotal), colors.disk)
		donutRow(c, st)
		statCards(c, st)
	}
	if hidden.shows(partNetwork) {
		v := st.Net(net.chosen)
		label, tip := netLabel(v)
		if v.Speed > 0 {
			label = fmt.Sprintf(locales.NetLinkSpeed(), label, strconv.FormatFloat(v.Speed, 'f', -1, 64))
		}
		var menu func(m *ui.Menu)
		if net.choose != nil {
			menu = func(m *ui.Menu) { netMenu(m, st, v, net) }
		}
		var rx, tx uint64
		if today != nil {
			rx, tx = today(v)
		}
		netCard(c, v, label, tip, menu, rx, tx)
	}
	if hidden.shows(partLocalNet) {
		localNetGraph(c, local)
	}
	if hidden.shows(partLatency) {
		latencyCard(c, st.Latency, st.LatHist)
	}
	if hidden.shows(partDisks) {
		section(c, locales.MonitorPartDisks(), func() {
			widgets.Muted(c, fmt.Sprintf(locales.Mounts(), len(st.Disks)))
		})
		diskTable(c, st.Disks)
	}
}

// netCard is the network part of the panel: the section with the link at
// its right, the download and upload speeds of the latest sample, the
// graph of both, and under it the traffic of the day and the graph's
// peak, as FinalShell's network card has them.
func netCard(c *ui.Context, v monitor.NetView, label, tip string, menu func(m *ui.Menu), todayRx, todayTx uint64) {
	var hint func()
	switch {
	case menu != nil:
		hint = func() { netLabelButton(c, label, tip, menu) }
	case label != "":
		hint = func() { widgets.Muted(c, label).SingleLine() }
	}
	section(c, locales.RemoteNet(), hint)
	ui.Row(c).Gap(10).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "↓ "+formatRate(v.RxRate)).TextColor(colors.rx).FontSize(12).Shrink(0)
		ui.Text(c, "↑ "+formatRate(v.TxRate)).TextColor(colors.tx).FontSize(12).Shrink(0)
		ui.Spacer(c)
		widgets.Muted(c, graphWindow()).FontSize(11)
	})
	widgets.Curve(c, monitor.HistoryLen, [][]float64{v.RxHist, v.TxHist}, []ui.Color{rxChart, txChart}, 56)
	ui.Row(c).Gap(8).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf(locales.NetToday(), formatBytes(float64(todayRx)), formatBytes(float64(todayTx)))).
			FontSize(11).TextColor(theme.TextMuted).SingleLine()
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf(locales.NetPeak(), formatRate(histPeak(v.RxHist, v.TxHist)))).
			FontSize(11).TextColor(theme.TextMuted).SingleLine()
	})
}

// latencyCard is the latency part, under the network's: the latest round
// trip and the graph's peak over the curve, in the graph's green.
func latencyCard(c *ui.Context, d time.Duration, hist []float64) {
	value := "-"
	if d > 0 {
		value = formatLatency(d)
	}
	ui.Row(c).Gap(8).Margin(8, 0, 2, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.Latency()).FontSize(12).TextColor(theme.TextMuted)
		ui.Text(c, value).FontSize(13).FontWeight(600).SingleLine()
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf(locales.NetPeak(), latencyPeak(hist))).
			FontSize(11).TextColor(theme.TextMuted).SingleLine()
	})
	widgets.Curve(c, monitor.HistoryLen, [][]float64{hist}, []ui.Color{latencyChart}, 46)
}

// latencyPeak is the graph's largest round trip, "-" for no sample yet.
func latencyPeak(hist []float64) string {
	if len(hist) == 0 {
		return "-"
	}
	return formatLatency(time.Duration(histPeak(hist) * float64(time.Millisecond)))
}

// graphWindow is how much of the past the graphs span: their samples, at
// the interval the panel reads the server.
func graphWindow() string {
	return fmt.Sprintf(locales.NetWindow(), int(monitor.Interval/time.Second)*monitor.HistoryLen)
}

// histPeak is the largest value of the series, 0 when there is none.
func histPeak(series ...[]float64) float64 {
	peak := 0.0
	for _, s := range series {
		for _, v := range s {
			peak = math.Max(peak, v)
		}
	}
	return peak
}

// section is one category of the panel: its title, the hint at its right
// when there is one, and the line under them that keeps the categories
// apart — the resources from the network from the processes.
func section(c *ui.Context, title string, hint func()) {
	ui.Row(c).Height(20).Gap(6).Margin(6, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(600).SingleLine()
		ui.Spacer(c)
		if hint != nil {
			hint()
		}
	})
	ui.Box(c).Height(1).Margin(0, 0, 2, 0).Background(colors.frame)
}

func ratio(a, b uint64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func usage(used, total uint64) string {
	return fmt.Sprintf("%.0f%%  %s/%s", 100*ratio(used, total), formatBytes(float64(used)), formatBytes(float64(total)))
}

// donutRow is the ring gauges under the bars: the CPU, the memory and
// the disks as a whole, which is what the bars cannot show — each of them
// reads one part at a glance, the three of them side by side.
func donutRow(c *ui.Context, st *monitor.Stats) {
	cpu, mem, disk := 0.0, 0.0, 0.0
	if st != nil {
		cpu = st.CPU / 100
		mem = ratio(st.MemUsed, st.MemTotal)
		disk = diskUsedRatio(st.Disks)
	}
	ui.Row(c).Gap(4).Margin(0, 2, 0, 0).Children(func() {
		widgets.Donut(c, locales.Cpu(), cpu, percentText(cpu), colors.cpuRing)
		widgets.Donut(c, locales.Memory(), mem, percentText(mem), colors.memRing)
		widgets.Donut(c, locales.SysDisks(), disk, percentText(disk), colors.diskRing)
	})
}

// percentText is a share, from 0 to 1, as the rings show it.
func percentText(frac float64) string {
	return fmt.Sprintf("%.0f%%", math.Max(0, math.Min(1, frac))*100)
}

// diskUsage is how much of the disks is used, their partitions summed:
// the bar and the ring show them as one disk, as the others show one
// server.
func diskUsage(disks []monitor.Disk) (used, total uint64) {
	for _, d := range disks {
		if d.Size > d.Avail {
			used += d.Size - d.Avail
		}
		total += d.Size
	}
	return used, total
}

// diskUsedRatio is how much of the disks is used, from 0 to 1.
func diskUsedRatio(disks []monitor.Disk) float64 {
	used, total := diskUsage(disks)
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total)
}

// statCards are the three numbers under the rings: the hottest sensor, the
// processes and those logged in — what the bars and the rings leave out.
func statCards(c *ui.Context, st *monitor.Stats) {
	temp, procs, users := "—", "—", "—"
	if st != nil {
		if st.TempC > 0 {
			temp = fmt.Sprintf("%.0f℃", st.TempC)
		}
		if st.ProcTotal > 0 {
			procs = strconv.Itoa(st.ProcTotal)
		}
		if st.Users > 0 {
			users = strconv.Itoa(st.Users)
		}
	}
	ui.Row(c).Gap(6).Margin(0, 2, 0, 0).Children(func() {
		monitorCard(c, locales.CpuTemp(), temp)
		monitorCard(c, locales.MonitorPartProcesses(), procs)
		monitorCard(c, locales.Users(), users)
	})
}

// monitorCard is one of them: its label over its number, small enough for
// the side panel, unlike the reports' statCard.
func monitorCard(c *ui.Context, label, value string) {
	ui.Column(c).Basis(0).Grow(1).Shrink(1).MinWidth(0).Gap(2).Padding(6).Radius(6).
		Border(1, colors.frame).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(theme.TextMuted).SingleLine()
		ui.Text(c, value).FontSize(15).FontWeight(600).SingleLine().Ellipsis("…")
	})
}

// localNetGraph is this computer's traffic, under the server's: the
// speeds of the latest sample, the graph of both directions and the peak,
// as the server's card has them.
func localNetGraph(c *ui.Context, lt *localTraffic) {
	up, down := 0.0, 0.0
	var upHist, downHist []float64
	if lt != nil {
		up, down = lt.up, lt.down
		upHist, downHist = lt.upHist, lt.downHist
	}
	section(c, locales.LocalNet(), func() { widgets.Muted(c, graphWindow()).FontSize(11) })
	ui.Row(c).Gap(10).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "↓ "+formatRate(down)).TextColor(colors.rx).FontSize(12).Shrink(0)
		ui.Text(c, "↑ "+formatRate(up)).TextColor(colors.tx).FontSize(12).Shrink(0)
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf(locales.NetPeak(), formatRate(histPeak(downHist, upHist)))).
			FontSize(11).TextColor(theme.TextMuted).SingleLine()
	})
	widgets.Curve(c, monitor.HistoryLen, [][]float64{downHist, upHist}, []ui.Color{rxChart, txChart}, 46)
}

func diskTable(c *ui.Context, disks []monitor.Disk) {
	ui.Column(c).Margin(6, 0, 0, 0).Radius(6).Border(1, colors.frame).Clip().Children(func() {
		ui.Row(c).PaddingX(6).Height(20).BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			ui.Text(c, locales.DiskPath()).Grow(1).FontSize(12)
			ui.Text(c, locales.DiskFreeSize()).FontSize(12)
		})
		for i, d := range disks {
			ui.Row(c.Key(i)).PaddingX(6).Height(19).Gap(6).Children(func() {
				ui.Text(c, d.Mount).FontSize(12).SingleLine().Grow(1).MinWidth(0)
				ui.Text(c, formatBytes(float64(d.Avail))+"/"+formatBytes(float64(d.Size))).FontSize(12)
			})
		}
	})
}

// sessionsPage is the list of open sessions, the page of the left column
// it opens on (conntree.go).
func (a *App) sessionsPage(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		// No Key: with one, the list follows the chosen session by it, and
		// chooses none once that session is gone, even though a.active
		// already holds the neighbor closeSession picked, which is on the
		// same row. a.active is the app's: closeSession and moveSessions
		// keep it on the right row.
		a.sessList.Selected = &a.active
		a.sessList.Label = func(i int) string { return listLabel(a.sessions[i]) }
		a.sessList.Reorder = a.moveSessions
		// A row asks to close its session, which closes once the list is
		// built: closing it in the row would leave the rows after it to
		// build with fewer sessions than the list was given.
		var closing *Session
		ui.List(c, &a.sessList, len(a.sessions), func(i int) {
			if a.sessionRow(c, a.sessions[i], i == a.active) {
				closing = a.sessions[i]
			}
		}).Grow(1).Padding(0, 4, 4, 4).Children(func() {
			if len(a.sessions) == 0 {
				widgets.Muted(c, locales.NoSessions()).Padding(8, 10)
			}
		})
		a.renameKey(c)
		if closing != nil {
			a.closeSession(closing)
		}
	})
}

// sessionCount is the number of sessions, local terminals included, new
// tabs and panel rows left out.
func (a *App) sessionCount() int {
	n := 0
	for _, s := range a.sessions {
		if !s.isNewTab() {
			n++
		}
	}
	return n
}

// sessionRow is the tab of s. It reports whether the user closed it.
func (a *App) sessionRow(c *ui.Context, s *Session, active bool) (closed bool) {
	if s.isPanelTab() {
		return a.panelRow(c, s, active)
	}
	row := ui.Row(c).Gap(8).Padding(5, 4, 5, 8).Radius(5)
	hovered := row.Hovered()
	if s.isNewTab() {
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.Close()).Danger().Chosen() {
				closed = true
			}
		})
		row.Children(func() {
			// As wide as a status dot, so that the names line up.
			ui.Box(c).Size(8, 8).Center().Shrink(0).Children(func() {
				ui.Icon(c, icons.Get("app-window")).FontSize(13).TextColor(rowSubColor(active))
			})
			closed = rowText(c, locales.NewTab(), locales.NotConnected(), active, hovered) || closed
		})
		return closed
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.Reconnect()).Chosen() {
			a.reconnect(s)
		}
		if m.Item(locales.DuplicateSession()).Chosen() {
			// In a tab of its own, though a new tab is selected.
			a.duplicateSession(s)
		}
		if m.Item(locales.Disconnect()).Disabled(s.status != statusConnected && s.status != statusConnecting).Chosen() {
			a.disconnectSession(s)
		}
		diag := s.status != statusConnected || s.isLocal() || s.isRedis()
		if m.Item(locales.DiagTitle()).Disabled(diag).Chosen() && !diag {
			a.openDiag(s)
		}
		if m.Item(locales.RecordFiles()).Chosen() {
			a.openRecFiles()
		}
		m.Separator()
		a.recordMenu(m, s)
		a.paneMenu(m, s)
		m.Separator()
		a.renameMenu(m, s)
		m.Separator()
		if m.Item(locales.Close()).Danger().Chosen() {
			closed = true
		}
	})
	// RowDoubleClicked, not DoubleClicked: that makes this row clickable,
	// and the press goes to the innermost clickable element, so the
	// list's row, which chooses the session, would never see a click.
	renaming := a.renaming(s)
	if row.RowDoubleClicked() && !renaming && s.status != statusConnected && s.status != statusConnecting {
		a.reconnect(s)
	}
	if s.name != "" && !renaming {
		row.Tooltip(renamedTooltip(s))
	}
	row.Children(func() {
		sessionMark(c, s, active)
		sub := s.target()
		switch {
		case s.status != statusConnected:
			sub = s.statusText()
		case s.isLocal() && s.programTitle() != "":
			sub = s.programTitle() // the program's own, as a terminal app's tab
		}
		sub += a.recordSub(s)
		name := func() { a.sessionName(c, s, active) }
		if renaming {
			name = func() { a.renameField(c, active) }
		}
		closed = rowLines(c, name, sub, active, hovered) || closed
	})
	return closed
}

// rowText is the name and the line under it of a session row, and its
// close button, which shows on the selected row and the row hovered. It
// reports whether the button was clicked.
func rowText(c *ui.Context, name, sub string, active, hovered bool) bool {
	return rowLines(c, func() { rowName(c, name, active) }, sub, active, hovered)
}

// rowLines is rowText with its name line built by name: the field of a
// row being renamed (rename.go).
func rowLines(c *ui.Context, name func(), sub string, active, hovered bool) bool {
	ui.Column(c).Grow(1).MinWidth(0).Children(func() {
		name()
		ui.Text(c, sub).FontSize(11.5).SingleLine().TextColor(rowSubColor(active))
	})
	closeBtn := widgets.IconButton(c, "x", locales.CloseSession()).Size(20, 20)
	if !hovered && !active {
		closeBtn.Invisible()
	}
	return closeBtn.Clicked()
}

// rowName is the name line of a session row.
func rowName(c *ui.Context, name string, active bool) ui.Element {
	t := ui.Text(c, name).SingleLine()
	if active {
		t.Bold()
	}
	return t
}

func rowSubColor(active bool) ui.Color {
	if active {
		return theme.AccentText.Alpha(0.8)
	}
	return theme.TextMuted
}
