package app

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// diagDialog is a connection's diagnostics: the round trip, the ports this
// computer can open to it, and what the server's own DNS and routing say,
// as QuickTerm shows them.
type diagDialog struct {
	s *Session

	mu       sync.Mutex
	latency  time.Duration
	latBusy  bool
	ports    []diagPort
	portBusy bool
	dns      string
	dnsBusy  bool
	hops     string
	hopBusy  bool
	msg      string
}

// diagPort is one port of the ports check.
type diagPort struct {
	port int
	ok   bool
	ms   int64
	note string
}

// diagPorts are the ports the check tries, as FinalShell's list has them.
var diagPorts = []int{22, 80, 443, 3306, 5432, 6379, 8080, 3389, 5900}

// diagTimeout is how long the server's own commands may take.
const diagTimeout = 12 * time.Second

// openDiag opens s's diagnostics, measuring the round trip at once, as
// QuickTerm does.
func (a *App) openDiag(s *Session) {
	if s == nil || s.status != statusConnected || s.Host == nil {
		a.notify(locales.DiagUnavailable())
		return
	}
	a.diag = &diagDialog{s: s}
	a.diagLatency()
}

// diagDialogs draws the diagnostics window while it is open.
func (a *App) diagDialogs(c *ui.Context) {
	d := a.diag
	if d == nil {
		return
	}
	open := true
	ui.Modal(c, &open, func() { a.diagView(c, d, &open) })
	if !open || d.s.status != statusConnected {
		a.diag = nil
	}
}

// diagLatency measures the round trip of a keepalive.
func (a *App) diagLatency() {
	d := a.diag
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.latBusy || d.s.client == nil {
		d.mu.Unlock()
		return
	}
	d.latBusy = true
	d.mu.Unlock()

	s := d.s
	go func() {
		start := time.Now()
		_, _, err := s.client.SendRequest("keepalive@openssh.com", true, nil)
		took := time.Since(start)
		a.post(func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if a.diag != d {
				return
			}
			d.latBusy = false
			if err == nil {
				d.latency = took
			}
		})
	}()
}

// diagCheckPorts tries each port from this computer, one at a time, as
// QuickTerm does: a device may refuse many sockets at once.
func (a *App) diagCheckPorts() {
	d := a.diag
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.portBusy {
		d.mu.Unlock()
		return
	}
	d.portBusy, d.ports = true, nil
	host, sessionPort := d.s.Host.Addr, d.s.Host.Port
	d.mu.Unlock()

	go func() {
		for _, port := range diagPorts {
			start := time.Now()
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 1200*time.Millisecond)
			p := diagPort{port: port, ok: err == nil, ms: time.Since(start).Milliseconds()}
			if conn != nil {
				_ = conn.Close()
			}
			if !p.ok {
				p.note = locales.DiagPortClosed()
			}
			if port == sessionPort {
				p.note = locales.DiagPortSession()
			}
			a.post(func() {
				d.mu.Lock()
				defer d.mu.Unlock()
				if a.diag == d {
					d.ports = append(d.ports, p)
				}
			})
		}
		a.post(func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if a.diag == d {
				d.portBusy = false
			}
		})
	}()
}

// diagDNS asks the server itself how it resolves the host.
func (a *App) diagDNS() {
	d := a.diag
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.dnsBusy || !safeHost(d.s.Host.Addr) || d.s.client == nil {
		d.mu.Unlock()
		return
	}
	d.dnsBusy, d.dns = true, ""
	host, client := d.s.Host.Addr, d.s.client
	d.mu.Unlock()

	script := "getent hosts " + host + " 2>/dev/null || nslookup " + host + " 2>/dev/null || host " + host + " 2>/dev/null || echo __NONE__\n"
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), diagTimeout)
		defer cancel()
		out, _, err := monitor.RunScript(ctx, client, script)
		a.post(func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if a.diag != d {
				return
			}
			d.dnsBusy = false
			text := strings.TrimSpace(out)
			switch {
			case ctx.Err() != nil:
				d.dns = locales.DiagTimeout()
			case err != nil && text == "":
				d.dns = locales.DiagFailed(describeError(err))
			case text == "" || strings.Contains(text, "__NONE__"):
				d.dns = locales.DiagDNSNone()
			default:
				d.dns = text
			}
		})
	}()
}

// diagHops asks the server for its route to the default gateway: the
// server's own view, as the app has no ICMP of its own.
func (a *App) diagHops() {
	d := a.diag
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.hopBusy || d.s.client == nil {
		d.mu.Unlock()
		return
	}
	d.hopBusy, d.hops = true, ""
	client := d.s.client
	d.mu.Unlock()

	script := `gw=$(ip route 2>/dev/null | awk '/^default/{print $3; exit}'); ` +
		`[ -z "$gw" ] && gw=$(netstat -rn 2>/dev/null | awk '/^default/{print $2; exit}'); ` +
		`[ -z "$gw" ] && gw=1.1.1.1; ` +
		`mtr -r -n -c 2 -m 15 "$gw" 2>/dev/null || traceroute -n -m 12 -q 1 -w 1 "$gw" 2>/dev/null || echo __NONE__` + "\n"
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), diagTimeout)
		defer cancel()
		out, _, err := monitor.RunScript(ctx, client, script)
		a.post(func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if a.diag != d {
				return
			}
			d.hopBusy = false
			text := strings.TrimSpace(out)
			switch {
			case ctx.Err() != nil:
				d.hops = locales.DiagTimeout()
			case err != nil && text == "":
				d.hops = locales.DiagFailed(describeError(err))
			case text == "" || strings.Contains(text, "__NONE__"):
				d.hops = locales.DiagHopsNone()
			default:
				d.hops = text
			}
		})
	}()
}

// diagAll runs the checks in the order of the window: this computer
// first, then the server.
func (a *App) diagAll() {
	a.diagLatency()
	a.diagCheckPorts()
	a.diagDNS()
	a.diagHops()
}

// diagText is what Copy puts on the clipboard: everything the window
// shows.
func (d *diagDialog) diagText() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var b strings.Builder
	h := d.s.Host
	fmt.Fprintf(&b, "%s: %s\n", locales.DiagSession(), d.s.name)
	fmt.Fprintf(&b, "%s: SSH\n", locales.DiagProtocol())
	fmt.Fprintf(&b, "%s: %s:%d\n", locales.DiagAddress(), h.Addr, h.Port)
	fmt.Fprintf(&b, "%s: %s\n", locales.DiagUser(), h.User)
	fmt.Fprintf(&b, "%s: %s\n", locales.DiagLatency(), diagLatencyText(d.latency, d.latBusy))
	ok := 0
	for _, p := range d.ports {
		if p.ok {
			ok++
		}
		fmt.Fprintf(&b, "  %d: %v %dms %s\n", p.port, p.ok, p.ms, p.note)
	}
	fmt.Fprintf(&b, "%s: %d/%d\n", locales.DiagPorts(), ok, len(d.ports))
	fmt.Fprintf(&b, "%s:\n%s\n", locales.DiagDNS(), d.dns)
	fmt.Fprintf(&b, "%s:\n%s\n", locales.DiagHops(), d.hops)
	return b.String()
}

// safeHost is a host that may go into a shell command: an address, a
// name, or an IPv6 address.
func safeHost(h string) bool {
	if h == "" || len(h) > 255 {
		return false
	}
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '-', r == '_', r == ':', r == '%':
		default:
			return false
		}
	}
	return true
}

// diagLatencyText says how the round trip reads.
func diagLatencyText(d time.Duration, busy bool) string {
	switch {
	case busy:
		return locales.DiagLatencyMeasuring()
	case d <= 0:
		return locales.DiagLatencyUnmeasured()
	}
	return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
}

// diagView is the window: the session, the checks, and their results.
func (a *App) diagView(c *ui.Context, d *diagDialog, open *bool) {
	h := d.s.Host
	ui.Column(c).Width(600).MaxHeight(680).Children(func() {
		ui.Row(c).Padding(12, 16).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			ui.Box(c).Size(3, 18).Radius(2).Background(theme.Accent).Shrink(0)
			ui.Text(c, locales.DiagTitle()).FontSize(15).Bold()
			widgets.Muted(c, d.s.name).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			if ui.Button(c, locales.Close()).Clicked() {
				*open = false
			}
		})
		ui.Scroll(c).MaxHeight(580).Children(func() {
			ui.Column(c).Padding(14, 16, 16, 16).Gap(14).Children(func() {
				procDialogSection(c, locales.DiagSession(), func() {
					procDialogField(c, locales.DiagProtocol(), "SSH")
					procDialogField(c, locales.DiagAddress(), net.JoinHostPort(h.Addr, strconv.Itoa(h.Port)))
					procDialogField(c, locales.DiagUser(), orDash(h.User))
					procDialogField(c, locales.DiagStatus(), d.s.statusText())
				})

				d.mu.Lock()
				lat, latBusy := d.latency, d.latBusy
				ports, portBusy := append([]diagPort(nil), d.ports...), d.portBusy
				dns, dnsBusy, hops, hopBusy := d.dns, d.dnsBusy, d.hops, d.hopBusy
				d.mu.Unlock()

				diagSection(c, locales.DiagLatency(), locales.DiagLatencyMeasure(), latBusy, func() { a.diagLatency() }, func() {
					ui.Text(c, diagLatencyText(lat, latBusy)).FontSize(12.5).Bold()
					widgets.Muted(c, locales.DiagLatencyNote()).FontSize(11.5)
				})

				diagSection(c, locales.DiagPorts(), locales.DiagPortsCheck(), portBusy, func() { a.diagCheckPorts() }, func() {
					ok := 0
					for _, p := range ports {
						if p.ok {
							ok++
						}
						ui.Row(c).Key(p.port).Gap(8).AlignItems(ui.Center).Children(func() {
							mark, color := "×", theme.Danger
							if p.ok {
								mark, color = "✓", colors.rx
							}
							ui.Text(c, fmt.Sprintf("%s %d", mark, p.port)).FontSize(12).TextColor(color)
							widgets.Muted(c, fmt.Sprintf("%d ms", p.ms)).FontSize(11.5)
							if p.note != "" {
								widgets.Muted(c, p.note).FontSize(11.5)
							}
						})
					}
					if len(ports) == 0 {
						widgets.Muted(c, locales.DiagPortsNone()).FontSize(12)
					} else {
						widgets.Muted(c, locales.DiagPortsSummary(strconv.Itoa(ok), strconv.Itoa(len(ports)))).FontSize(12)
					}
				})

				diagSection(c, locales.DiagDNS(), locales.DiagDNSRun(), dnsBusy, func() { a.diagDNS() }, func() {
					diagResult(c, dnsBusy, dns)
				})
				diagSection(c, locales.DiagHops(), locales.DiagHopsRun(), hopBusy, func() { a.diagHops() }, func() {
					diagResult(c, hopBusy, hops)
				})
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(10, 16, 12, 16).
			BorderWidth(1, 0, 0, 0).BorderColor(colors.line).Children(func() {
			if ui.Button(c, locales.DiagAll()).Clicked() {
				a.diagAll()
			}
			widgets.Muted(c, locales.DiagAllNote()).FontSize(11.5).Shrink(1).MinWidth(0)
			ui.Spacer(c)
			if ui.Button(c, locales.DiagCopy()).Clicked() {
				c.WriteClipboard(d.diagText())
				a.notify(locales.DiagCopied())
			}
		})
	})
}

// diagSection is a check's card: its title, its button, and its result.
func diagSection(c *ui.Context, title, action string, busy bool, run func(), body func()) {
	procDialogSection(c, title, func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, action).Disabled(busy).Clicked() {
				run()
			}
		})
		ui.Column(c).Gap(4).Children(body)
	})
}

// diagResult is a check's text, with a spinner while it runs.
func diagResult(c *ui.Context, busy bool, text string) {
	if busy {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Label(locales.ReadingServerInfo())
			widgets.Muted(c, locales.DiagRunning())
		})
		return
	}
	if text == "" {
		widgets.Muted(c, locales.DiagLatencyUnmeasured()).FontSize(12)
		return
	}
	ui.Text(c, text).Selectable().FontSize(11.5).Wrap()
}
