package app

import (
	"slices"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The one-off command window: a script run on a host without a terminal. A
// session connected to the host runs it on a new channel of its connection;
// another host is dialed with the saved secret and closed after. The output
// is there to read, select and copy, as the terminal's is.

// runCmdState is the window's state: the host chosen, by ID, the script, what
// running wrote and failed with, and whether it runs now.
type runCmdState struct {
	host   string
	script string
	out    string
	err    string
	// who is the title of the host the output is from, "" before a run; clipped
	// says the output was cut to its last runCmdOutLimit bytes.
	who     string
	clipped bool
	busy    bool
}

// runCmdOutLimit is how much of a script's output the window shows: the end of
// it, as a terminal shows.
const runCmdOutLimit = 128 << 10

// runCmdWindow is its spec: the script and its output stack, so taller than
// the plugin center.
var runCmdWindow = WindowSpec{
	Kind:      WindowRunCmd,
	Title:     locales.RunCmdTitle,
	Width:     820,
	Height:    580,
	MinWidth:  640,
	MinHeight: 420,
}

// openRunCmd opens the window, choosing a host for it the first time: the one
// in front, or the first saved.
func (a *App) openRunCmd() {
	if !a.windowOpen(WindowRunCmd) {
		a.pickRunCmdHost()
	}
	a.openWindow(runCmdWindow, a.runCmdView)
}

// pickRunCmdHost keeps the host chosen while it is still saved, and chooses
// the one in front, or else the first saved, for a window that has none.
func (a *App) pickRunCmdHost() {
	if h := a.hosts.Host(a.runCmd.host); h != nil && !h.Deleted() {
		return
	}
	a.runCmd.host = ""
	if s := a.current(); s != nil && s.Host != nil {
		a.runCmd.host = s.Host.ID
		return
	}
	if live := a.hosts.Live(); len(live) > 0 {
		a.runCmd.host = live[0].ID
	}
}

// hostLabels are the hosts' titles as the window's list shows them: a title
// two hosts share takes their target too, so that a row picks one of them.
func hostLabels(hosts []*config.Host) []string {
	shared := map[string]int{}
	for _, h := range hosts {
		shared[h.Title()]++
	}
	labels := make([]string, len(hosts))
	for i, h := range hosts {
		labels[i] = h.Title()
		if shared[labels[i]] > 1 {
			labels[i] += " · " + h.Target()
		}
	}
	return labels
}

// runCmdView is the window: the note, the host and the script with the button
// that runs it, then the output.
func (a *App) runCmdView(c *ui.Context) {
	ui.Column(c).Fill().Padding(12, 14, 12, 14).Gap(10).Children(func() {
		ui.Text(c, locales.RunCmdNote()).FontSize(11).TextColor(theme.TextMuted).MaxLines(3)
		hosts := a.hosts.Live()
		if len(hosts) == 0 {
			widgets.Muted(c, locales.RunCmdNoHosts())
			return
		}
		a.runCmdHostRow(c, hosts)
		ui.Field(c, locales.RunCmdScript(), func() {
			ui.TextArea(c, &a.runCmd.script).Font("monospace").Lines(4, 8).FillWidth()
		})
		a.runCmdOutput(c)
	})
}

// runCmdHostRow is the host to run on and the button, which waits for a
// script and for the run before it.
func (a *App) runCmdHostRow(c *ui.Context, hosts []*config.Host) {
	labels := hostLabels(hosts)
	i := slices.IndexFunc(hosts, func(h *config.Host) bool { return h.ID == a.runCmd.host })
	if i < 0 {
		// The host chosen was deleted while the window was open.
		i = 0
		a.runCmd.host = hosts[0].ID
	}
	chosen := labels[i]
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if ui.Select(c, &chosen, labels).Label(locales.RunCmdHost()).Width(320).Changed() {
			if i := slices.Index(labels, chosen); i >= 0 {
				a.runCmd.host = hosts[i].ID
			}
		}
		ui.Spacer(c)
		label := locales.RunCmdRun()
		if a.runCmd.busy {
			label = locales.RunCmdRunning()
		}
		ready := !a.runCmd.busy && strings.TrimSpace(a.runCmd.script) != ""
		if ui.PrimaryButton(c, label).Disabled(!ready).Clicked() {
			a.runRunCmd()
		}
	})
}

// runCmdOutput is what the last run wrote, with its host, what it failed with,
// and the note of a clipped output.
func (a *App) runCmdOutput(c *ui.Context) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.RunCmdOutput()).FontSize(11).TextColor(theme.TextMuted)
		if a.runCmd.who != "" {
			ui.Text(c, a.runCmd.who).FontSize(11).SingleLine().MinWidth(0)
		}
		ui.Spacer(c)
		if a.runCmd.err != "" {
			ui.Text(c, a.runCmd.err).FontSize(11).TextColor(theme.Danger).SingleLine()
		}
	})
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		if a.runCmd.who == "" {
			widgets.Muted(c, locales.RunCmdIdle())
			return
		}
		if a.runCmd.clipped {
			ui.Text(c, locales.RunCmdClipped(formatSize(runCmdOutLimit))).FontSize(10).
				TextColor(theme.TextMuted)
		}
		ui.Text(c, a.runCmd.out).Font("monospace").FontSize(11).Selectable()
	})
}

// runRunCmd runs the script on the host chosen, and shows what it wrote when
// the goroutine that runs it calls back on the UI thread.
func (a *App) runRunCmd() {
	if a.runCmd.busy {
		return
	}
	script := a.runCmd.script
	if strings.TrimSpace(script) == "" {
		return
	}
	h := a.hosts.Host(a.runCmd.host)
	if h == nil || h.Deleted() {
		a.runCmd.err = locales.RunCmdNoHosts()
		return
	}
	a.runCmd.busy, a.runCmd.out, a.runCmd.err, a.runCmd.clipped = true, "", "", false
	a.runCmd.who = h.Title()
	a.runOnHost(h, script, func(out string, err error) {
		a.runCmd.busy = false
		a.runCmd.err = describeError(err)
		a.runCmd.out, a.runCmd.clipped = clipRunCmd(out)
	})
}

// clipRunCmd is the output the window shows: all of it, or its last
// runCmdOutLimit bytes from the start of a line, and whether it took any away.
func clipRunCmd(out string) (string, bool) {
	if len(out) <= runCmdOutLimit {
		return out, false
	}
	cut := out[len(out)-runCmdOutLimit:]
	if i := strings.IndexByte(cut, '\n'); i >= 0 && i+1 < len(cut) {
		cut = cut[i+1:]
	}
	// The cut may fall in the middle of a character: its tail goes with the
	// bytes left out.
	for len(cut) > 0 && cut[0]&0xC0 == 0x80 {
		cut = cut[1:]
	}
	return cut, true
}
