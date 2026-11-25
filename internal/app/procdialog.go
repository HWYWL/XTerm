package app

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// procDetailDialog is the process details as a dialog, opened by a double
// click on a row, as QuickTerm shows them: what the process is, its causal
// chain, its command line and paths, and what may be done with it.
type procDetailDialog struct {
	s   *Session
	p   monitor.Process // the row's values: the CPU and memory of the list
	tab *procTab        // what reads the details, as the tab below the list

	// The causal chain, read apart from the details so the tab below the
	// list does not pay for it.
	chain        monitor.Causality
	chainLoading bool
	chainErr     error
	chainGen     int
	chainCancel  context.CancelFunc

	msg  string // the last action's receipt
	fail bool
	busy bool
}

// openProcDialog opens p's details as a dialog, and reads them.
func (a *App) openProcDialog(s *Session, p monitor.Process) {
	a.closeProcDialog()
	d := &procDetailDialog{s: s, p: p, tab: &procTab{pid: p.PID, name: p.Name, start: p.Start, envRow: -1}}
	a.processes.detail = d
	a.readProcTab(s, d.tab)
	a.readProcChain(d)
}

// closeProcDialog closes the dialog, stopping its reads.
func (a *App) closeProcDialog() {
	if d := a.processes.detail; d != nil {
		if d.tab.cancel != nil {
			d.tab.cancel()
		}
		if d.chainCancel != nil {
			d.chainCancel()
		}
	}
	a.processes.detail = nil
}

// readProcChain traces d's process on a goroutine: the processes that
// started it, from init down.
func (a *App) readProcChain(d *procDetailDialog) {
	if d.chainCancel != nil {
		d.chainCancel()
	}
	d.chainGen++
	if d.s.status != statusConnected {
		d.chainLoading, d.chainErr, d.chainCancel = false, errNoClient, nil
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), procTimeout)
	gen, client, api := d.chainGen, d.s.client, a.procAPI()
	d.chainLoading, d.chainErr, d.chainCancel = true, nil, cancel
	go func() {
		chain, err := api.causality(ctx, client, d.p.PID)
		if ctx.Err() == context.DeadlineExceeded {
			err = context.DeadlineExceeded
		}
		cancel()
		a.post(func() {
			if d.chainGen != gen {
				return
			}
			d.chainLoading, d.chainCancel = false, nil
			d.chain, d.chainErr = chain, err
		})
	}()
}

// procDetailDialogs draws the details dialog while it is open.
func (a *App) procDetailDialogs(c *ui.Context) {
	d := a.processes.detail
	if d == nil {
		return
	}
	open := true
	ui.Modal(c, &open, func() { a.procDetailDialogView(c, d, &open) })
	if !open || d.s.status != statusConnected || !slices.Contains(a.sessions, d.s) {
		if a.processes.detail == d {
			a.closeProcDialog()
		}
	}
}

// procDetailDialogView is the dialog: the process's numbers, its command
// line, its paths, and the buttons that act on it.
func (a *App) procDetailDialogView(c *ui.Context, d *procDetailDialog, open *bool) {
	p, t := d.p, d.tab
	connected := d.s.status == statusConnected
	ui.Column(c).Width(560).MaxWidth(680).Children(func() {
		ui.Row(c).Padding(12, 16).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			ui.Box(c).Size(3, 18).Radius(2).Background(theme.Accent).Shrink(0)
			ui.Text(c, locales.ProcDialogTitle()).FontSize(15).Bold().Shrink(0)
			widgets.Muted(c, p.Name+" · PID "+strconv.Itoa(p.PID)).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			if ui.Button(c, locales.Close()).Clicked() {
				*open = false
			}
		})
		ui.Scroll(c).MaxHeight(520).Children(func() {
			ui.Column(c).Padding(14, 16, 16, 16).Gap(14).Children(func() {
				procDialogSection(c, locales.ProcDialogSection(), func() {
					procDialogField(c, locales.ColPID(), strconv.Itoa(p.PID))
					procDialogField(c, locales.ColProcUser(), orDash(t.d.User))
					procDialogField(c, locales.ColProcCPU(), fmt.Sprintf("%.1f%%", p.CPU))
					procDialogField(c, locales.ColProcMemory(), formatSize(int64(p.RSS)))
					procDialogField(c, locales.ProcDialogState(), procStateText(t.d.State))
				})
				procDialogSection(c, locales.ProcDialogChain(), func() {
					switch {
					case d.chainLoading:
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Spinner(c).Label(locales.ProcChainReading())
							widgets.Muted(c, locales.ProcChainReading())
						})
					case d.chainErr != nil:
						ui.Text(c, describeProcError(d.chainErr)).TextColor(theme.Danger).FontSize(12).Wrap()
					case !d.chain.Found:
						ui.Text(c, procChainReason(d.chain.Reason)).FontSize(12).TextColor(theme.TextMuted).Wrap()
					default:
						ui.Column(c).Gap(5).Children(func() {
							for i, n := range d.chain.Nodes {
								procChainNode(c, n, i)
							}
						})
						if len(d.chain.Warnings) > 0 {
							ui.Column(c).Gap(3).Children(func() {
								for _, w := range d.chain.Warnings {
									procChainWarning(c, w)
								}
							})
						}
					}
				})
				procDialogSection(c, locales.ProcDialogCommand(), func() {
					ui.Text(c, orDash(p.Args)).Selectable().FontSize(12.5).Wrap()
				})
				procDialogSection(c, locales.ProcDialogPaths(), func() {
					procDialogFieldDenied(c, locales.ColProcPath(), orDash(t.d.Path), t.d.PathDenied)
					procDialogFieldDenied(c, locales.ProcCwd(), orDash(t.d.Cwd), t.d.CwdDenied)
				})
				procDialogSection(c, locales.ProcDialogActions(), func() {
					ui.Row(c).Gap(8).Wrap().Children(func() {
						if ui.Button(c, locales.ProcCopyCommand()).Clicked() {
							c.WriteClipboard(p.Args)
							d.msg, d.fail = locales.ProcDialogCopied(), false
						}
						if ui.Button(c, locales.ProcDialogLowerPriority()).Disabled(!connected || d.busy).Clicked() {
							a.reniceProcess(d)
						}
						if ui.Button(c, locales.ProcKill()).Disabled(!connected).Clicked() {
							a.closeProcDialog()
							a.askSignal(d.s, p, "TERM", true)
							return
						}
						if ui.Button(c, "SIGKILL · "+locales.SigKILLShort()).Clicked() {
							a.closeProcDialog()
							a.askSignal(d.s, p, "KILL", false)
							return
						}
					})
					if d.busy {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Spinner(c).Label(locales.ReadingServerInfo())
							widgets.Muted(c, locales.ReadingServerInfo())
						})
					}
					if d.msg != "" {
						color := theme.TextMuted
						if d.fail {
							color = theme.Danger
						}
						ui.Text(c, d.msg).FontSize(12).TextColor(color).Wrap()
					}
					switch {
					case t.loading && t.d.PID == 0:
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Spinner(c).Label(locales.ReadingServerInfo())
							widgets.Muted(c, locales.ReadingServerInfo())
						})
					case t.err != nil && t.d.PID == 0:
						ui.Text(c, procDetailsError(t)).TextColor(theme.Danger).FontSize(12).Wrap()
					}
				})
			})
		})
	})
}

// procDialogSection is a titled card of the dialog.
func procDialogSection(c *ui.Context, title string, body func()) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		widgets.Muted(c, title).FontSize(11.5)
		ui.Column(c).FillWidth().Gap(6).Padding(10, 12).Radius(8).
			Background(colors.sidebar).Border(1, colors.line).Children(body)
	})
}

// procDialogField is a label and a value on a line.
func procDialogField(c *ui.Context, label, value string) {
	procDialogFieldDenied(c, label, value, false)
}

// procDialogFieldDenied is procDialogField, saying when the value needs
// root to be read.
func procDialogFieldDenied(c *ui.Context, label, value string, denied bool) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		widgets.Muted(c, label).Width(96).Shrink(0).FontSize(12)
		ui.Row(c).Gap(8).Grow(1).MinWidth(0).Wrap().Children(func() {
			ui.Text(c, value).FontSize(12.5).Selectable()
			if denied {
				widgets.Muted(c, locales.ProcNeedsRoot()).FontSize(11)
			}
		})
	})
}

// procStateText says what the process's state letter means.
func procStateText(state byte) string {
	switch state {
	case 'R':
		return locales.ProcStateRunning()
	case 'S', 'I':
		return locales.ProcStateSleeping()
	case 'D':
		return locales.ProcStateWaiting()
	case 'Z':
		return locales.ProcStateZombie()
	case 'T', 't':
		return locales.ProcStateStopped()
	}
	return "-"
}

// procChainNode is a process on the causal chain: its PID, where it came
// from, who runs it and how long it has run, and its command, indented by
// its depth under the root.
func procChainNode(c *ui.Context, n monitor.CausalityNode, depth int) {
	marker, color := "·", theme.TextMuted
	if n.IsTarget {
		marker, color = "▸", theme.Accent
	}
	ui.Column(c).Gap(1).PaddingStart(float32(depth) * 10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, marker).FontSize(11).TextColor(color).Shrink(0)
			ui.Text(c, strconv.Itoa(n.PID)).FontSize(12).Bold().TextColor(color).Shrink(0)
			widgets.Muted(c, procChainKind(n.Kind)).FontSize(11).Shrink(0)
			ui.Spacer(c)
			ui.Text(c, procChainFacts(n)).FontSize(11).TextColor(theme.TextMuted).Shrink(1).SingleLine()
		})
		ui.Text(c, orDash(n.Command)).FontSize(12).PaddingStart(20).SingleLine()
	})
}

// procChainFacts is a node's owner, state, age and memory, on one line.
func procChainFacts(n monitor.CausalityNode) string {
	return n.User + " · " + procStateText(n.State) + " · " + procChainAge(n.Age) + " · " + procChainRSS(n.RSS)
}

func procChainAge(seconds uint64) string {
	if seconds == 0 {
		return "-"
	}
	return formatUptime(time.Duration(seconds) * time.Second)
}

func procChainRSS(rss uint64) string {
	if rss == 0 {
		return "-"
	}
	return formatSize(int64(rss))
}

// procChainWarning is what the chain says of the target, in the color of
// how much it matters.
func procChainWarning(c *ui.Context, w monitor.CausalityWarning) {
	color := theme.TextMuted
	switch w.Level {
	case "danger":
		color = theme.Danger
	case "warn":
		color = theme.Warning
	}
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, "·").FontSize(11).TextColor(color).Shrink(0)
		ui.Text(c, procChainWarningText(w)).FontSize(11.5).TextColor(color).Grow(1).MinWidth(0).Wrap()
	})
}

// procChainKind says where a process on the chain came from.
func procChainKind(kind string) string {
	switch kind {
	case "init":
		return locales.ProcChainKindInit()
	case "ssh":
		return locales.ProcChainKindSSH()
	case "cron":
		return locales.ProcChainKindCron()
	case "container":
		return locales.ProcChainKindContainer()
	case "multiplexer":
		return locales.ProcChainKindMultiplexer()
	case "shell":
		return locales.ProcChainKindShell()
	case "orphan":
		return locales.ProcChainKindOrphan()
	}
	return locales.ProcChainKindProc()
}

// procChainWarningText is a warning of the chain, in words.
func procChainWarningText(w monitor.CausalityWarning) string {
	switch w.Code {
	case "zombie":
		return locales.ProcChainWarnZombie()
	case "stopped":
		return locales.ProcChainWarnStopped()
	case "uninterruptible":
		return locales.ProcChainWarnUninterruptible()
	case "root":
		return locales.ProcChainWarnRoot()
	case "long-running":
		return locales.ProcChainWarnLongRunning(w.Arg)
	case "big-rss":
		return locales.ProcChainWarnBigRSS(w.Arg)
	case "container":
		return locales.ProcChainWarnContainer()
	case "ssh":
		return locales.ProcChainWarnSSH()
	case "adopted":
		return locales.ProcChainWarnAdopted()
	case "no-init":
		return locales.ProcChainWarnNoInit()
	case "non-root":
		return locales.ProcChainWarnNonRoot()
	case "no-cwd":
		return locales.ProcChainWarnNoCwd()
	case "injected":
		return locales.ProcChainWarnInjected(w.Arg)
	}
	return w.Code
}

// procChainReason says why there is no chain.
func procChainReason(reason string) string {
	switch reason {
	case "gone":
		return locales.ProcChainReasonGone()
	case "not-linux":
		return locales.ProcChainReasonNotLinux()
	}
	return locales.ProcChainReasonNoData()
}

// reniceProcess raises d's process's nice value on a goroutine, so it
// yields to others, and shows how it went.
func (a *App) reniceProcess(d *procDetailDialog) {
	s := d.s
	if d.busy || s.status != statusConnected {
		return
	}
	d.busy, d.msg, d.fail = true, "", false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), procTimeout)
		defer cancel()
		err := monitor.SetNice(ctx, s.client, d.p.PID)
		a.post(func() {
			d.busy = false
			if err != nil {
				d.msg, d.fail = locales.ProcDialogNiceFailed(describeProcError(err)), true
			} else {
				d.msg, d.fail = locales.ProcDialogReniced(), false
			}
		})
	}()
}
