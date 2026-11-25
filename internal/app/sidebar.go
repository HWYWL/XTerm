package app

import (
	"fmt"
	"math"
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
		serverStats(c, st, a.monitorHidden(), a.netPicker(s), &a.localNet, a.openProcesses)
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
// choice of network interfaces, and a click on the process list calls
// openProcs.
func serverStats(c *ui.Context, st *monitor.Stats, hidden hiddenParts, net netPicker, local *localTraffic, openProcs func()) {
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
			widgets.UsageBar(c, locales.Cpu(), 0, "0%", colors.cpu)
			widgets.UsageBar(c, locales.Memory(), 0, "0%", colors.mem)
			widgets.UsageBar(c, locales.Swap(), 0, "0%", colors.swap)
			donutRow(c, nil)
		}
		if hidden.shows(partProcesses) {
			procTable(c, nil, 0, nil)
		}
		if hidden.shows(partNetwork) {
			netRow(c, 0, 0, "", "", nil)
			widgets.Sparkline(c, monitor.HistoryLen, nil, nil, formatBytes)
		}
		if hidden.shows(partLocalNet) {
			localNetGraph(c, nil)
		}
		if hidden.shows(partLatency) {
			latencyRow(c, "-")
			widgets.Sparkline(c, monitor.HistoryLen, nil, nil, formatMillis)
		}
		if hidden.shows(partDisks) {
			diskTable(c, nil)
		}
		return
	}
	if hidden.shows(partUptime) {
		ui.Text(c, locales.UptimeDays(int(st.Uptime.Hours()/24)))
		ui.Text(c, locales.LoadAverage(fmt.Sprintf("%.2f, %.2f, %.2f", st.Load[0], st.Load[1], st.Load[2])))
	}
	if hidden.shows(partCPU) {
		widgets.UsageBar(c, locales.Cpu(), st.CPU/100, fmt.Sprintf("%.0f%%", st.CPU), colors.cpu)
		widgets.UsageBar(c, locales.Memory(), ratio(st.MemUsed, st.MemTotal), usage(st.MemUsed, st.MemTotal), colors.mem)
		widgets.UsageBar(c, locales.Swap(), ratio(st.SwapUsed, st.SwapTotal), usage(st.SwapUsed, st.SwapTotal), colors.swap)
		donutRow(c, st)
	}
	if hidden.shows(partProcesses) {
		procTable(c, st.Procs, st.MemTotal, openProcs)
	}
	if hidden.shows(partNetwork) {
		netGraph(c, st, net)
	}
	if hidden.shows(partLocalNet) {
		localNetGraph(c, local)
	}
	if hidden.shows(partLatency) {
		latencyRow(c, formatLatency(st.Latency))
		widgets.Sparkline(c, monitor.HistoryLen, [][]float64{st.LatHist}, []ui.Color{latencyChart}, formatMillis)
	}
	if hidden.shows(partDisks) {
		diskTable(c, st.Disks)
	}
}

func formatMillis(v float64) string { return fmt.Sprintf("%.0f", v) }

// netRow is the speeds above the network graph and, at the right, label:
// the button of the interfaces' menu when there is a menu.
func netRow(c *ui.Context, tx, rx float64, label, tip string, menu func(m *ui.Menu)) {
	ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
		ui.Text(c, "↑"+formatBytes(tx)).TextColor(colors.tx).Shrink(0)
		ui.Text(c, "↓"+formatBytes(rx)).TextColor(colors.rx).Shrink(0)
		ui.Spacer(c)
		if menu != nil {
			netLabelButton(c, label, tip, menu)
		} else {
			widgets.Muted(c, label)
		}
	})
}

func latencyRow(c *ui.Context, latency string) {
	ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
		ui.Text(c, latency).TextColor(theme.Accent)
		ui.Spacer(c)
		widgets.Muted(c, locales.Latency())
	})
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

// diskUsedRatio is how much of the disks is used, their partitions
// summed: the ring shows them as one disk, as the bars show one server.
func diskUsedRatio(disks []monitor.Disk) float64 {
	var used, total uint64
	for _, d := range disks {
		if d.Size > d.Avail {
			used += d.Size - d.Avail
		}
		total += d.Size
	}
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total)
}

// localNetGraph is this computer's traffic, under the server's: the
// speeds, then the graph of both directions, as the server's is.
func localNetGraph(c *ui.Context, lt *localTraffic) {
	up, down := 0.0, 0.0
	var upHist, downHist []float64
	if lt != nil {
		up, down = lt.up, lt.down
		upHist, downHist = lt.upHist, lt.downHist
	}
	ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
		ui.Text(c, "↑"+formatBytes(up)).TextColor(colors.tx).Shrink(0)
		ui.Text(c, "↓"+formatBytes(down)).TextColor(colors.rx).Shrink(0)
		ui.Spacer(c)
		widgets.Muted(c, locales.LocalNet())
	})
	widgets.Sparkline(c, monitor.HistoryLen, [][]float64{upHist, downHist}, []ui.Color{txChart, rxChart}, formatBytes)
}

// procTable is the processes using the most memory; a click opens the
// process manager with open, when it is not nil. Its rows are the CPU
// share, the memory share of memTotal, and the command, over a bar as
// wide as the CPU share, as QuickTerm's list has them.
func procTable(c *ui.Context, procs []monitor.Proc, memTotal uint64, open func()) {
	headText := colors.procHeadText
	table := ui.Column(c).Border(1, colors.frame).Background(colors.procTable)
	if open != nil {
		table.Cursor(ui.CursorPointer).Tooltip(locales.OpenProcesses()).Role(ui.RoleButton).Label(locales.OpenProcesses())
		if table.Clicked() {
			open()
		}
	}
	table.Children(func() {
		ui.Row(c).Background(colors.procHead).PaddingX(6).Height(20).Children(func() {
			ui.Text(c, locales.Cpu()+"%").Width(44).TextColor(headText).FontSize(12)
			ui.Text(c, locales.Memory()+"%").Width(44).TextColor(headText).FontSize(12)
			ui.Text(c, locales.Command()).TextColor(headText).FontSize(12)
		})
		for i, p := range procs {
			proc := p
			ui.Box(c).Key(i).Height(22).Clip().Children(func() {
				ui.Box(c).Absolute().Top(0).Left(0).Bottom(0).
					WidthPercent(float32(math.Max(0, math.Min(1, proc.CPU/100)) * 100)).Background(colors.cpu)
				ui.Row(c).Fill().PaddingX(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("%.1f", proc.CPU)).Width(44).FontSize(12)
					ui.Text(c, fmt.Sprintf("%.1f", procMemShare(proc, memTotal))).Width(44).FontSize(12)
					ui.Text(c, proc.Name).FontSize(12).SingleLine().Grow(1).MinWidth(0)
				})
			})
		}
	})
}

// procMemShare is how much of the memory a process holds, in percent.
func procMemShare(p monitor.Proc, memTotal uint64) float64 {
	if memTotal == 0 {
		return 0
	}
	return float64(p.RSS) / float64(memTotal) * 100
}

func diskTable(c *ui.Context, disks []monitor.Disk) {
	ui.Column(c).Margin(6, 0, 0, 0).Border(1, colors.frame).Background(colors.diskTable).Children(func() {
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
		m.Separator()
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
