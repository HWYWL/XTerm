package app

import (
	"fmt"
	"strconv"
	"time"

	"finallyshell/internal/adb"
	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// adbPanel is the System page of an ADB tab: the link and its versions, the
// traffic and its timing, the ADB packets and channels, the log stream, and
// the device's own numbers when they are collected (adbmeter.go).
func (a *App) adbPanel(c *ui.Context, s *Session) {
	m := a.adbMeter(s)
	v := m.view()
	h := s.Host
	link := adbLinkState(s, v)
	ui.Column(c).FillWidth().Padding(10, 10, 8, 10).Gap(6).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			a.hostIcon(c, h, 16, theme.Text)
			ui.Text(c, h.Title()).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			widgets.StatusDot(c, telnetLinkColor(telnetLinkOfText(link)))
			ui.Text(c, link).FontSize(11).TextColor(telnetLinkColor(telnetLinkOfText(link))).Shrink(0)
		})
		widgets.Muted(c, h.Target()).FontSize(11).SingleLine()

		// The device's own numbers, collected every few seconds: what the
		// phone is doing, before what the connection is.
		section(c, locales.AdbMetricsTitle(), nil)
		metricsOn := v.Metrics
		if ui.Checkbox(c, &metricsOn, locales.AdbMetricsOn()).FontSize(11).Changed() {
			m.setMetrics(metricsOn)
		}
		if v.Metrics {
			adbDeviceRows(c, v)
		}

		// The link: the device, how it is reached, and the versions.
		section(c, locales.TelnetLink(), nil)
		serialRow(c, locales.TelnetState(), link, telnetLinkColor(telnetLinkOfText(link)))
		device := v.Serial
		if device == "" {
			device = "—" // a device on the network has no serial written on it
		}
		mode := v.Mode
		if mode == "" {
			if h.ADBConfig().Wireless() {
				mode = locales.AdbWireless() + " " + h.Target()
			} else {
				mode = locales.AdbWired()
			}
		}
		serialRow(c, locales.AdbSerial(), device, theme.Text)
		serialRow(c, locales.AdbConnection(), mode, theme.Text)
		if v.State != "" {
			serialRow(c, locales.AdbState(), v.State, adbStateColor(v.State))
		}
		version := v.ST.ClientVersion
		if version == 0 {
			version = adb.ClientVersion
		}
		serialRow(c, locales.AdbProtocol(), "client "+adbVersionText(version), theme.TextMuted)
		serialRow(c, locales.TelnetReconnects(), strconv.Itoa(v.Reconnects), theme.Text)

		// The traffic, as the serial and Telnet pages have it.
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
				serialCount(c, locales.AdbTimeouts(), v.ST.Timeouts, true)
				serialCount(c, locales.AdbIOErrs(), v.ST.ReadErrs+v.ST.WriteErrs, true)
				serialRow(c, locales.AdbLastData(), adbAgo(v.LastFrame), theme.TextMuted)
			})
		serialPair(c,
			func() {
				serialRow(c, locales.AdbMaxIdle(), adbSeconds(v.MaxGap), theme.TextMuted)
				serialRow(c, locales.AdbGap(), adbSeconds(v.LastGap), theme.TextMuted)
			},
			func() {
				serialRow(c, locales.AdbPackets(), formatCount(float64(v.ST.Packets)), theme.Text)
			})

		// The ADB packets the connection opened and closed, and its
		// channels, which is where a leak shows.
		section(c, locales.AdbPacketsTitle(), nil)
		serialPair(c,
			func() {
				serialCount(c, "CNXN", v.ST.CNXN, true)
				serialCount(c, "OPEN", v.ST.OPEN, true)
				serialCount(c, "OKAY", v.ST.OKAY, true)
			},
			func() {
				serialCount(c, "CLSE", v.ST.CLSE, true)
				serialCount(c, "WRTE", v.ST.Packets, true)
				serialCount(c, locales.AdbAbnormal(), v.ST.Failures+v.ST.Malformed, true)
			})
		serialRow(c, locales.AdbActiveChannels(),
			fmt.Sprintf(locales.AdbChannelCount(), v.ST.Active), adbChannelColor(v.ST))
		serialRow(c, "shell · exec · logcat",
			fmt.Sprintf("%d · %d · %d", v.ST.ActiveByChannel[0], v.ST.ActiveByChannel[1],
				v.ST.ActiveByChannel[2]), theme.TextMuted)
		serialRow(c, "sync · jdwp",
			fmt.Sprintf("%d · %d", v.ST.ActiveByChannel[3], v.ST.ActiveByChannel[4]), theme.TextMuted)

		// The log stream, which reads the device's log over a channel of
		// its own while it is on.
		section(c, locales.AdbLogTitle(), func() {
			if v.Logcat {
				widgets.Muted(c, locales.AdbLogHint()).FontSize(10.5)
			}
		})
		logOn := v.Logcat
		if ui.Checkbox(c, &logOn, locales.AdbLogOn()).FontSize(11).Changed() {
			m.setLogcat(s, logOn)
		}
		if v.Logcat {
			serialPair(c,
				func() {
					serialRow(c, locales.AdbLogAlive(), adbYesNo(v.ST.Logcat.Alive), theme.TextMuted)
					serialRow(c, locales.AdbLogLines(), formatCount(float64(v.ST.Logcat.Lines)), theme.Text)
				},
				func() {
					serialRow(c, locales.AdbLogBytes(), formatBytes(float64(v.ST.Logcat.Bytes)), theme.Text)
					serialCount(c, locales.AdbLogErrors(),
						v.ST.Logcat.E+v.ST.Logcat.F, true)
				})
			serialRow(c, "V · D · I · W · E · F",
				fmt.Sprintf("%d · %d · %d · %d · %d · %d",
					v.ST.Logcat.V, v.ST.Logcat.D, v.ST.Logcat.I,
					v.ST.Logcat.W, v.ST.Logcat.E, v.ST.Logcat.F), theme.TextMuted)
		}
	})
}

// adbDeviceRows are the device's own numbers, as the poll reported them.
func adbDeviceRows(c *ui.Context, v adbView) {
	if v.DeviceErr != "" && !v.DeviceOK {
		widgets.Muted(c, v.DeviceErr).FontSize(11).Wrap()
		return
	}
	if !v.DeviceOK {
		widgets.Muted(c, locales.AdbCollecting()).FontSize(11)
		return
	}
	d := v.Device
	serialRow(c, locales.AdbModel(), d.Model, theme.Text)
	serialRow(c, locales.AdbAndroid(), d.Android+" · SDK "+d.SDK, theme.Text)
	serialRow(c, locales.AdbUptime(), adbUptime(d.Uptime), theme.Text)
	serialRow(c, locales.Cpu(), adbPercent(d.CPU), theme.Text)
	serialRow(c, locales.Memory(), fmt.Sprintf("%s / %s",
		formatBytes(float64(d.MemTotal-d.MemFree)), formatBytes(float64(d.MemTotal))), theme.Text)
	if d.DataTotal > 0 {
		serialRow(c, locales.AdbStorage(), fmt.Sprintf("%s / %s",
			formatBytes(float64(d.DataUsed)), formatBytes(float64(d.DataTotal))), theme.Text)
	}
	serialRow(c, locales.AdbProcs(), formatCount(float64(d.Procs)), theme.Text)
	bat := fmt.Sprintf("%d%%", d.Battery)
	if d.Charging {
		bat += " · " + locales.AdbCharging()
	}
	if d.BatteryTemp > 0 {
		bat += fmt.Sprintf(" · %.1f°C", d.BatteryTemp)
	}
	serialRow(c, locales.AdbBattery(), bat, theme.Text)
	if d.SoCTemp > 0 {
		serialRow(c, locales.AdbSoCTemp(), fmt.Sprintf("%.1f°C", d.SoCTemp), theme.Text)
	}
	serialRow(c, locales.AdbRoot(), adbYesNo(d.Root), theme.Text)
	if since := time.Since(v.DeviceAt); since > 0 && !v.DeviceAt.IsZero() {
		widgets.Muted(c, fmt.Sprintf(locales.AdbSampled(), adbSeconds(since))).FontSize(10.5)
	}
}

// setLogcat starts or stops reading the device's log.
func (m *adbMeter) setLogcat(s *Session, on bool) {
	m.mu.Lock()
	m.logcat = on
	conn, ctx := m.conn, s.ctx
	if ctx == nil {
		ctx = s.ctx
	}
	m.mu.Unlock()
	if !on || conn == nil || ctx == nil {
		return
	}
	conn.WatchLogcat(ctx)
}

// setMetrics turns the collection of the device's numbers on or off.
func (m *adbMeter) setMetrics(on bool) {
	m.mu.Lock()
	m.metrics = on
	if on {
		m.devAt = time.Time{} // due at once
	}
	m.mu.Unlock()
}

// adbStateColor is the device's state as the ADB server says it: green
// when it is a device, red when it is not.
func adbStateColor(state string) ui.Color {
	if state == "device" {
		return assetGreen
	}
	return assetOrange
}

// adbChannelColor is red once a channel failed, or one is left behind.
func adbChannelColor(st adb.Stats) ui.Color {
	if st.Failures > 0 || st.Malformed > 0 {
		return assetRed
	}
	return theme.Text
}

// adbYesNo is a switch as the page writes it.
func adbYesNo(on bool) string {
	if on {
		return "✓"
	}
	return "—"
}

// adbVersionText is an ADB protocol version as adb writes it: 1.0.1.
func adbVersionText(v uint32) string {
	return fmt.Sprintf("%d.%d.%d", v>>24, (v>>16)&0xff, v&0xffff)
}

// adbUptime writes a device's uptime as days, hours and minutes.
func adbUptime(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}

// adbAgo is when something last happened, or a dash before it did.
func adbAgo(at time.Time) string {
	if at.IsZero() {
		return "—"
	}
	return fmt.Sprintf(locales.SerialAgo(), adbSeconds(time.Since(at)))
}

// telnetLinkOfText is the link's colour from its words, which the ADB page
// borrows the Telnet page's.
func telnetLinkOfText(text string) telnetLink {
	switch text {
	case locales.TelnetConnected():
		return telnetLinkConnected
	case locales.StatusConnecting():
		return telnetLinkConnecting
	case locales.TelnetReadTimeout(), locales.TelnetConnectTimeout():
		return telnetLinkReadTimeout
	case locales.TelnetDisconnected():
		return telnetLinkDisconnected
	}
	return telnetLinkWriteFailed
}
