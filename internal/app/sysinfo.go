package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/icons"
	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"
)

// sysInfoTimeout is how long the dialog waits for the server.
const sysInfoTimeout = 20 * time.Second

// sysInfoState is the System info dialog: the server's own details, as
// QuickTerm shows them, in cards. There is one, for the session shown; each
// open reads the server again.
type sysInfoState struct {
	open    bool
	s       *Session // the session it shows
	gen     int      // counts the reads; a result of an older one is dropped
	loading bool
	info    monitor.SysInfo
	err     error
	off     bool // the host allows no exec channel
	cancel  context.CancelFunc
}

// execAllowed tells whether the app may open exec channels to h's server
// besides the shell and SFTP: the server monitor and the system info.
// It is the one place that decides, by the host's Monitor switch; a
// session without a host (a local terminal) allows none.
func execAllowed(h *config.Host) bool { return h != nil && h.MonitorEnabled() }

var errNoClient = errors.New("not connected")

// sysInfoReader reads a server's system info over client.
type sysInfoReader func(ctx context.Context, client *ssh.Client) (monitor.SysInfo, error)

// readSysInfo reads what the dialog shows, unless App.readSysInfo stands in
// for it, as in tests.
func readSysInfo(ctx context.Context, client *ssh.Client) (monitor.SysInfo, error) {
	if client == nil {
		return monitor.SysInfo{}, errNoClient
	}
	return monitor.ReadSysInfo(ctx, client)
}

// openSysInfo opens the dialog for s and starts reading the server on a
// goroutine, when its host allows it.
func (a *App) openSysInfo(s *Session) {
	a.closeSysInfo()
	p := &a.sysInfo
	p.open, p.s = true, s
	p.info, p.err, p.loading, p.off = monitor.SysInfo{}, nil, false, false
	if !execAllowed(s.Host) {
		p.off = true
		return
	}
	if s.status != statusConnected {
		return
	}
	read := a.readSysInfo
	if read == nil {
		read = readSysInfo
	}
	ctx, cancel := context.WithTimeout(context.Background(), sysInfoTimeout)
	p.loading, p.cancel = true, cancel
	gen, client := p.gen, s.client
	go func() {
		info, err := read(ctx, client)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = context.DeadlineExceeded
		}
		cancel()
		a.post(func() {
			// Closed, opened again, or showing another session since.
			if p.gen != gen || p.s != s {
				return
			}
			p.loading, p.info, p.err, p.cancel = false, info, err, nil
		})
	}()
}

// closeSysInfo closes the dialog, dropping the read under way.
func (a *App) closeSysInfo() {
	p := &a.sysInfo
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.open, p.loading, p.s = false, false, nil
	p.gen++
}

// sysInfoButton is the button at the end of the IP's line: it opens the
// dialog, and closes it. It is disabled while s is nil or not connected.
func (a *App) sysInfoButton(c *ui.Context, s *Session) {
	p := &a.sysInfo
	// Another session shown, or this one closed: the dialog was its.
	if p.open && (s == nil || p.s != s) {
		a.closeSysInfo()
	}
	enabled := s != nil && s.status == statusConnected
	b := ui.Button(c, locales.SystemInfo()).FontSize(12).Padding(2, 6).Disabled(!enabled)
	if b.Clicked() && enabled {
		if p.open {
			a.closeSysInfo()
		} else {
			a.openSysInfo(s)
		}
	}
}

// sysInfoDialog draws the system info dialog while it is open, as a modal of
// the window: dialogs calls it. Escape, the Close button, or a press outside
// it closes it, as with the others.
func (a *App) sysInfoDialog(c *ui.Context) {
	p := &a.sysInfo
	if !p.open {
		return
	}
	open := true
	// As tall as the window allows, the cards scrolling inside, as
	// QuickTerm's panel.
	w, h := c.Size()
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(min(w-96, 720)).Height(min(h-120, 640)).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(0, 0, 10, 0).Children(func() {
				ui.Text(c, locales.SystemInfo()).FontSize(16).Bold()
				ui.Spacer(c)
				ready := !p.off && !p.loading && p.err == nil && p.s != nil && p.s.status == statusConnected
				cp := sysInfoBarButton(c, "copy", locales.Copy()).Disabled(!ready)
				if cp.Clicked() && ready {
					a.copyToClipboard(c, sysInfoSummary(p.info))
				}
				if sysInfoBarButton(c, "x", locales.SysClose()).Clicked() {
					open = false
				}
			})
			switch {
			case p.off:
				widgets.Muted(c, locales.SysInfoOff())
			case p.loading:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 0).Children(func() {
					ui.Spinner(c).Label(locales.ReadingServerInfo())
					widgets.Muted(c, locales.ReadingServerInfo())
				})
			case p.err != nil:
				ui.Column(c).Gap(8).Children(func() {
					ui.Text(c, locales.SysInfoFailed(describeSysInfoError(p.err))).TextColor(theme.Danger).Selectable()
					if p.s != nil && p.s.status == statusConnected && ui.Button(c, locales.Retry()).AlignSelf(ui.Start).Clicked() {
						a.openSysInfo(p.s)
					}
				})
			case p.s == nil || p.s.status != statusConnected:
				widgets.Muted(c, locales.NotConnected())
			default:
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					a.sysInfoCards(c)
				})
			}
		})
	})
	if !open {
		a.closeSysInfo()
	}
}

// sysInfoCards is the dialog's body: the overview, then a card of each
// table, as QuickTerm's.
func (a *App) sysInfoCards(c *ui.Context) {
	info := a.sysInfo.info
	ui.Column(c).Gap(10).Children(func() {
		sysInfoOverview(c, info)
		sysInfoCPU(c, info)
		sysInfoGPU(c, info)
		sysInfoUsage(c, info)
		sysInfoMemSwap(c, info)
		sysInfoNets(c, info)
		sysInfoFS(c, info)
	})
}

// sysInfoBarButton is a button of the dialog's title bar: an icon and a
// label in a box of the same size for each, so that Copy and Close line up.
func sysInfoBarButton(c *ui.Context, iconName, label string) ui.Element {
	b := ui.Button(c, "").FontSize(12).Height(26).MinWidth(78).Padding(0, 10).Label(label)
	b.Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Get(iconName)).FontSize(12)
			ui.Text(c, label).FontSize(12)
		})
	})
	return b
}

// sysInfoCard is a card of the dialog: its title over what fn builds.
func sysInfoCard(c *ui.Context, title string, fn func()) {
	ui.Column(c).Radius(6).Border(1, theme.Border).Clip().Children(func() {
		ui.Row(c).Padding(7, 10, 7, 10).Background(colors.sidebar).Children(func() {
			ui.Text(c, title).FontSize(11).Bold()
		})
		fn()
	})
}

// sysInfoTable draws a table of the dialog: head, then rows, each cell
// weighted as weights says.
func sysInfoTable(c *ui.Context, head []string, weights []float32, rows [][]string) {
	line := func(cells []string, header bool) {
		ui.Row(c).Gap(8).Padding(5, 10, 5, 10).AlignItems(ui.Start).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.rowLine).Children(func() {
			for i := range head {
				cell := ""
				if i < len(cells) {
					cell = cells[i]
				}
				t := ui.Text(c, cell).FontSize(11).MinWidth(0).SingleLine().Ellipsis("…")
				if w := weights[i]; w > 0 {
					t = t.Grow(w)
				}
				if header {
					t = t.TextColor(theme.TextMuted)
				}
			}
		})
	}
	line(head, true)
	for _, r := range rows {
		line(r, false)
	}
}

// sysInfoEmpty says a card has nothing to show, as QuickTerm does.
func sysInfoEmpty(c *ui.Context) {
	widgets.Muted(c, locales.SysNoData()).FontSize(11).Padding(7, 10)
}

// sysInfoPair is a labelled value of the overview: the label in muted, the
// value beside it.
func sysInfoPair(c *ui.Context, k, v string) {
	ui.Row(c).Gap(6).Grow(1).MinWidth(0).AlignItems(ui.Start).Children(func() {
		widgets.Muted(c, k).Width(92).Shrink(0)
		ui.Text(c, v).FontSize(11).Grow(1).MinWidth(0).Ellipsis("…")
	})
}

func sysInfoOverview(c *ui.Context, info monitor.SysInfo) {
	or := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	up := info.UptimeUp
	if up == "" {
		up = formatUptime(info.Uptime)
	}
	rows := [][4]string{
		{locales.SysOS(), or(info.OS), locales.SysKernel(), or(info.Kernel)},
		{locales.SysHostname(), or(info.Hostname), locales.SysIPs(), or(info.IPs)},
		{locales.SysLoad(), or(info.Load), locales.SysArch(), or(info.Arch)},
		{locales.SysUptime(), or(up), "", ""},
	}
	sysInfoCard(c, locales.SysOverview(), func() {
		for _, r := range rows {
			ui.Row(c).Gap(16).Padding(5, 10, 5, 10).
				BorderWidth(0, 0, 1, 0).BorderColor(colors.rowLine).Children(func() {
				sysInfoPair(c, r[0], r[1])
				if r[2] != "" {
					sysInfoPair(c, r[2], r[3])
				}
			})
		}
	})
}

func sysInfoCPU(c *ui.Context, info monitor.SysInfo) {
	or := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	cells := []string{or(info.CPUModel), or(num(info.CPUs)), or(info.CPUFreq), or(info.CPUCache), or(info.CPUBogo)}
	sysInfoCard(c, locales.SysCPU(), func() {
		sysInfoTable(c,
			[]string{locales.SysColName(), locales.SysColCores(), locales.SysColFreq(), locales.SysColCache(), locales.SysColBogo()},
			[]float32{4, 1, 1.2, 1.2, 1.2},
			[][]string{cells})
	})
}

// sysInfoGPU is lspci's display controller: what it is, and what the app
// does not read of it, as QuickTerm leaves them too.
func sysInfoGPU(c *ui.Context, info monitor.SysInfo) {
	sysInfoCard(c, locales.SysGpu(), func() {
		if info.GPU == "" {
			sysInfoEmpty(c)
			return
		}
		sysInfoTable(c,
			[]string{locales.SysColName(), locales.SysColVendor(), locales.SysColDriver(), locales.SysColMemory()},
			[]float32{4, 2, 2, 2},
			[][]string{{info.GPU, "-", "-", "-"}})
	})
}

func sysInfoUsage(c *ui.Context, info monitor.SysInfo) {
	u := info.Usage
	other := fmt.Sprintf("IO %.1f%% / IRQ %.1f%% / SoftIRQ %.1f%% / Steal %.1f%%", u.IOWait, u.IRQ, u.SoftIRQ, u.Steal)
	sysInfoCard(c, locales.SysCpuUsage(), func() {
		sysInfoTable(c,
			[]string{locales.SysColUser(), locales.SysColSystem(), locales.SysColNice(), locales.SysColIdle(), locales.SysColOther()},
			[]float32{1, 1, 1, 1, 3},
			[][]string{{pct(u.User), pct(u.System), pct(u.Nice), pct(u.Idle), other}})
	})
}

func sysInfoMemSwap(c *ui.Context, info monitor.SysInfo) {
	memUsed, memPct := usedOf(info.Mem.Total, info.Mem.Available)
	swapUsed, swapPct := usedOf(info.Swap.Total, info.Swap.Free)
	memRow := []string{formatSize(int64(info.Mem.Total)), formatSize(int64(memUsed)), formatSize(int64(info.Mem.Free)), pct(memPct), formatSize(int64(info.Mem.CachedTotal()))}
	swapRow := []string{formatSize(int64(info.Swap.Total)), formatSize(int64(swapUsed)), formatSize(int64(info.Swap.Free)), pct(swapPct)}
	ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			sysInfoCard(c, locales.SysMemory(), func() {
				sysInfoTable(c,
					[]string{locales.SysColTotal(), locales.SysColUsed(), locales.SysColFree(), locales.SysColUsage(), locales.SysColCache()},
					[]float32{1, 1, 1, 1, 1},
					[][]string{memRow})
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			sysInfoCard(c, locales.Swap(), func() {
				sysInfoTable(c,
					[]string{locales.SysColTotal(), locales.SysColUsed(), locales.SysColFree(), locales.SysColUsage()},
					[]float32{1, 1, 1, 1},
					[][]string{swapRow})
			})
		})
	})
}

func sysInfoNets(c *ui.Context, info monitor.SysInfo) {
	sysInfoCard(c, locales.SysNetwork(), func() {
		if len(info.Nets) == 0 {
			sysInfoEmpty(c)
			return
		}
		var rows [][]string
		for _, n := range info.Nets {
			rows = append(rows, []string{n.Name, formatSize(int64(n.Tx)), formatSize(int64(n.Rx)), "-", "-"})
		}
		sysInfoTable(c,
			[]string{locales.SysColName(), locales.SysColSent(), locales.SysColRecv(), locales.SysColSentRate(), locales.SysColRecvRate()},
			[]float32{1.5, 1, 1, 1, 1},
			rows)
	})
}

func sysInfoFS(c *ui.Context, info monitor.SysInfo) {
	sysInfoCard(c, locales.SysFilesystems(), func() {
		if len(info.FS) == 0 {
			sysInfoEmpty(c)
			return
		}
		var rows [][]string
		for _, f := range info.FS {
			rows = append(rows, []string{f.Name, formatSize(int64(f.Size)), formatSize(int64(f.Used)), formatSize(int64(f.Avail)), f.Mount})
		}
		sysInfoTable(c,
			[]string{locales.SysColName(), locales.SysColSize(), locales.SysColUsed(), locales.SysColAvail(), locales.SysColMount()},
			[]float32{2, 1, 1, 1, 1.5},
			rows)
	})
}

// usedOf is the used part of total, with avail free, and its percent.
func usedOf(total, avail uint64) (uint64, float64) {
	if total <= avail {
		return 0, 0
	}
	used := total - avail
	return used, float64(used) / float64(total) * 100
}

func pct(v float64) string { return fmt.Sprintf("%.1f%%", v) }

func num(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	days := int(d / (24 * time.Hour))
	rest := d % (24 * time.Hour)
	if days > 0 {
		return locales.SysUptimeValue(days, fmt.Sprintf("%d:%02d", int(rest/time.Hour), int(rest%time.Hour/time.Minute)))
	}
	return fmt.Sprintf("%d:%02d", int(rest/time.Hour), int(rest%time.Hour/time.Minute))
}

func describeSysInfoError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return locales.SysInfoTimeout()
	case errors.Is(err, monitor.ErrSysInfoOutput):
		return locales.SysInfoNoShell()
	case errors.Is(err, errNoClient):
		return locales.NotConnected()
	}
	return describeError(err)
}

// sysInfoSummary is what Copy puts on the clipboard: the overview's rows,
// then each table, as plain text.
func sysInfoSummary(info monitor.SysInfo) string {
	or := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	line := func(b *strings.Builder, k, v string) {
		fmt.Fprintf(b, "%s: %s\n", k, or(v))
	}
	var b strings.Builder
	line(&b, locales.SysHostname(), info.Hostname)
	line(&b, locales.SysOS(), info.OS)
	line(&b, locales.SysKernel(), info.Kernel)
	line(&b, locales.SysArch(), info.Arch)
	line(&b, locales.SysLoad(), info.Load)
	line(&b, locales.SysIPs(), info.IPs)
	if info.UptimeUp != "" {
		line(&b, locales.SysUptime(), info.UptimeUp)
	} else {
		line(&b, locales.SysUptime(), formatUptime(info.Uptime))
	}
	fmt.Fprintf(&b, "\n%s:\n  %s\n", locales.SysCPU(), or(info.CPUModel))
	if info.GPU != "" {
		fmt.Fprintf(&b, "%s: %s\n", locales.SysGpu(), info.GPU)
	}
	u := info.Usage
	fmt.Fprintf(&b, "\n%s:\n  %s %s  %s %s  %s %s  %s %s\n", locales.SysCpuUsage(),
		locales.SysColUser(), pct(u.User), locales.SysColSystem(), pct(u.System),
		locales.SysColNice(), pct(u.Nice), locales.SysColIdle(), pct(u.Idle))
	_, memPct := usedOf(info.Mem.Total, info.Mem.Available)
	fmt.Fprintf(&b, "\n%s: %s %s (%s)\n", locales.SysMemory(), formatSize(int64(info.Mem.Total)), locales.SysColUsed(), pct(memPct))
	if info.Swap.Total > 0 {
		_, swapPct := usedOf(info.Swap.Total, info.Swap.Free)
		fmt.Fprintf(&b, "%s: %s %s (%s)\n", locales.Swap(), formatSize(int64(info.Swap.Total)), locales.SysColUsed(), pct(swapPct))
	}
	if len(info.Nets) > 0 {
		fmt.Fprintf(&b, "\n%s:\n", locales.SysNetwork())
		for _, n := range info.Nets {
			fmt.Fprintf(&b, "  %s  %s %s  %s %s\n", n.Name, locales.SysColSent(), formatSize(int64(n.Tx)), locales.SysColRecv(), formatSize(int64(n.Rx)))
		}
	}
	if len(info.FS) > 0 {
		fmt.Fprintf(&b, "\n%s:\n", locales.SysFilesystems())
		for _, f := range info.FS {
			fmt.Fprintf(&b, "  %s  %s  %s\n", f.Name, formatSize(int64(f.Used)), f.Mount)
		}
	}
	return b.String()
}
