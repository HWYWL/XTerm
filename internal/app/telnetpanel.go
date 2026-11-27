package app

import (
	"fmt"
	"net"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// telnetPanel is the System page of a Telnet tab: the link and what the
// server negotiated, what the connection carried, the errors the socket
// saw, and the IAC commands of the stream (telnetmeter.go samples them
// once a second).
func (a *App) telnetPanel(c *ui.Context, s *Session) {
	m := a.telnetMeter(s)
	v := m.view()
	h := s.Host
	link := telnetLinkOf(s, v)
	ui.Column(c).FillWidth().Padding(10, 10, 8, 10).Gap(6).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			a.hostIcon(c, h, 16, theme.Text)
			ui.Text(c, h.Title()).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			widgets.StatusDot(c, telnetLinkColor(link))
			ui.Text(c, telnetLinkText(link)).FontSize(11).
				TextColor(telnetLinkColor(link)).Shrink(0)
		})
		widgets.Muted(c, h.Target()).FontSize(11).SingleLine()

		// The link: its state, where it goes, what the server negotiated,
		// and whether the heartbeat still comes back.
		section(c, locales.TelnetLink(), nil)
		serialRow(c, locales.TelnetState(), telnetLinkText(link), telnetLinkColor(link))
		remoteHost, remotePort := telnetAddrParts(v.Addr, h.AddrPort(23))
		serialRow(c, locales.TelnetRemote(), remoteHost, theme.Text)
		serialRow(c, locales.TelnetPort(), remotePort, theme.Text)
		serialPair(c,
			func() {
				serialRow(c, "ECHO", telnetOptionText(v.Opts.Echo), telnetOptionColor(v.Opts.Echo))
				serialRow(c, "LINEMODE", telnetOptionText(v.Opts.Linemode),
					telnetOptionColor(v.Opts.Linemode))
			},
			func() {
				serialRow(c, "SGA", telnetOptionText(v.Opts.SGA), telnetOptionColor(v.Opts.SGA))
				serialRow(c, locales.TelnetIdle(), telnetIdle(v), telnetIdleColor(v))
			})
		serialRow(c, locales.TelnetHeartbeat(),
			fmt.Sprintf(locales.TelnetBeats(), v.Beats, v.BeatsOK), telnetBeatColor(v))
		serialRow(c, locales.TelnetLastBeat(), telnetAgo(v.LastBeatOK), theme.TextMuted)

		// The traffic: the rates of the last second, the curves, the
		// totals, and the peaks of the session.
		section(c, locales.SerialTraffic(), func() {
			widgets.Muted(c, graphWindow()).FontSize(11)
		})
		ui.Row(c).Gap(10).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "↓ "+formatRate(v.RXRate)).TextColor(colors.rx).FontSize(12).Shrink(0)
			ui.Text(c, "↑ "+formatRate(v.TXRate)).TextColor(colors.tx).FontSize(12).Shrink(0)
			ui.Spacer(c)
			if peak := max(v.PeakRX, v.PeakTX); peak > 0 {
				widgets.Muted(c, fmt.Sprintf(locales.NetPeak(), formatRate(peak))).FontSize(11)
			}
		})
		widgets.Curve(c, monitor.HistoryLen, [][]float64{v.RXHist, v.TXHist},
			[]ui.Color{rxChart, txChart}, 46)
		serialPair(c,
			func() {
				serialRow(c, "↓ "+locales.SerialTotal(), formatBytes(float64(v.RX)), colors.rx)
				serialRow(c, "↑ "+locales.SerialTotal(), formatBytes(float64(v.TX)), colors.tx)
			},
			func() {
				serialRow(c, locales.TelnetSessionTotal(),
					formatBytes(float64(v.RX+v.TX)), theme.Text)
				serialRow(c, locales.TelnetPeaks(),
					"↓"+formatRate(v.PeakRX)+" ↑"+formatRate(v.PeakTX), theme.Text)
			})

		// The socket's errors: the timeouts, the reset, the refusals and
		// the attempts the session needed.
		section(c, locales.TelnetNetErrors(), nil)
		serialPair(c,
			func() {
				serialCount(c, locales.TelnetReadTimeouts(), v.ReadTimeouts, true)
				serialCount(c, locales.TelnetResets(), v.ST.Resets, true)
				serialCount(c, locales.TelnetConnectTimeouts(), uint64(v.DialTimeouts), true)
			},
			func() {
				serialCount(c, locales.TelnetWriteTimeouts(), v.WriteTimeouts, true)
				serialCount(c, locales.TelnetRefused(), uint64(v.Refused), true)
				serialCount(c, locales.TelnetReconnects(), uint64(v.Reconnects), true)
			})
		halfColor := theme.Text
		if v.HalfOpens > 0 {
			halfColor = assetRed
		}
		serialRow(c, locales.TelnetHalfOpens(), formatCount(float64(v.HalfOpens)), halfColor)

		// The protocol: the IAC commands of the stream, what each side
		// negotiated, and the sequences a broken server sends.
		section(c, locales.TelnetProto(), nil)
		widgets.Muted(c, locales.TelnetIAC()).FontSize(10.5)
		serialPair(c,
			func() {
				serialCount(c, "NOP", v.ST.NOP, true)
				serialCount(c, "GA", v.ST.GA, true)
				serialCount(c, "ECHO", v.ST.Echo, true)
				serialCount(c, "SB", v.ST.Subneg, true)
			},
			func() {
				serialCount(c, "WILL", v.ST.WILL, true)
				serialCount(c, "WONT", v.ST.WONT, true)
				serialCount(c, "DO", v.ST.DO, true)
				serialCount(c, "DONT", v.ST.DONT, true)
			})
		widgets.Muted(c, locales.TelnetNegoSent()).FontSize(10.5)
		serialPair(c,
			func() {
				serialCount(c, "WILL", v.ST.SentWILL, true)
				serialCount(c, "WONT", v.ST.SentWONT, true)
			},
			func() {
				serialCount(c, "DO", v.ST.SentDO, true)
				serialCount(c, "DONT", v.ST.SentDONT, true)
			})
		serialPair(c,
			func() {
				serialCount(c, locales.TelnetBadIAC(), v.ST.InvalidIAC, true)
			},
			func() {
				serialCount(c, locales.TelnetBadSub(), v.ST.Malformed, true)
			})
	})
}

// telnetAddrParts is the far end of the connection as its address and its
// port: the address it was dialled at, or the host's while it is not
// connected.
func telnetAddrParts(addr, fallback string) (host, port string) {
	if addr == "" {
		addr = fallback
	}
	if h, p, err := net.SplitHostPort(addr); err == nil {
		return h, p
	}
	return addr, "—"
}

// telnetLinkText names the link as the row shows it.
func telnetLinkText(l telnetLink) string {
	switch l {
	case telnetLinkConnecting:
		return locales.StatusConnecting()
	case telnetLinkConnected:
		return locales.TelnetConnected()
	case telnetLinkConnectTimeout:
		return locales.TelnetConnectTimeout()
	case telnetLinkReadTimeout:
		return locales.TelnetReadTimeout()
	case telnetLinkWriteFailed:
		return locales.TelnetWriteFailed()
	}
	return locales.TelnetDisconnected()
}

// telnetLinkColor is the link's colour: green while it carries bytes, red
// once it failed or went quiet.
func telnetLinkColor(l telnetLink) ui.Color {
	switch l {
	case telnetLinkConnected:
		return assetGreen
	case telnetLinkConnecting:
		return assetOrange
	case telnetLinkDisconnected:
		return theme.TextMuted
	}
	return assetRed
}

// telnetBeatColor is the heartbeat's colour: muted until one went out, red
// once one went unanswered.
func telnetBeatColor(v telnetView) ui.Color {
	if v.BeatMisses > 0 || v.HalfOpen {
		return assetRed
	}
	if v.BeatsOK > 0 {
		return assetGreen
	}
	return theme.TextMuted
}

// telnetIdleColor turns red once the socket has been quiet past the point
// the page takes for a dead link.
func telnetIdleColor(v telnetView) ui.Color {
	if v.HalfOpen || v.Idle > telnetIdleWarn {
		return assetRed
	}
	if v.Idle > telnetIdleWarn/2 {
		return assetOrange
	}
	return theme.TextMuted
}

// telnetIdle is how long the socket has been quiet, or a dash before it
// has waited at all.
func telnetIdle(v telnetView) string {
	if v.Idle <= 0 {
		return "—"
	}
	return fmt.Sprintf(locales.SerialAgo(), serialGap(v.Idle))
}

// telnetAgo is when something happened, or a dash before it did.
func telnetAgo(at time.Time) string {
	if at.IsZero() {
		return "—"
	}
	return fmt.Sprintf(locales.SerialAgo(), serialGap(time.Since(at)))
}
