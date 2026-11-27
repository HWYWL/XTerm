package app

import (
	"fmt"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// serialPanel is the System page of a serial tab: the line's link and pins,
// what the port carried and how fast, the errors the line and the app saw,
// the buffers, the frames' timing, and the lines the device printed
// (serialmeter.go samples them once a second).
func (a *App) serialPanel(c *ui.Context, s *Session) {
	m := a.serialMeter(s)
	v := m.view()
	h := s.Host
	ui.Column(c).FillWidth().Padding(10, 10, 8, 10).Gap(6).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			a.hostIcon(c, h, 16, theme.Text)
			ui.Text(c, h.Title()).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			widgets.StatusDot(c, serialStatusColor(s))
			ui.Text(c, s.statusText()).FontSize(11).TextColor(serialStatusColor(s)).Shrink(0)
		})
		st := h.SerialPort()
		widgets.Muted(c, fmt.Sprintf("%s · %d %d %s %s", h.Target(), st.Baud, st.DataBits, st.Parity, st.StopBits)).
			FontSize(11).SingleLine()

		// The link: the state, the line's settings, the output pins the
		// app raises, and the input pins the device does.
		section(c, locales.SerialLink(), nil)
		serialRow(c, locales.SerialStatus(), s.statusText(), serialStatusColor(s))
		serialRow(c, locales.SerialLineState(), serialLineState(v), serialLineStateColor(v))
		serialRow(c, locales.SerialLine(), fmt.Sprintf("%d %d %s %s",
			st.Baud, st.DataBits, st.Parity, st.StopBits), theme.TextMuted)
		ui.Row(c).Gap(6).Children(func() {
			a.serialPinToggle(c, m, locales.SerialDTR(), v.DTR, m.setDTR)
			a.serialPinToggle(c, m, locales.SerialRTS(), v.RTS, m.setRTS)
		})
		serialPair(c,
			func() {
				serialModemPin(c, locales.SerialDSR(), v.Bits.DSR, v.BitsOK)
				serialModemPin(c, locales.SerialDCD(), v.Bits.DCD, v.BitsOK)
			},
			func() {
				serialModemPin(c, locales.SerialCTS(), v.Bits.CTS, v.BitsOK)
				serialModemPin(c, locales.SerialRI(), v.Bits.RI, v.BitsOK)
			})

		// The traffic: the rates of the last second, the curves, and the
		// totals the port and the day have carried.
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
				serialRow(c, "↓ "+locales.SerialToday(), formatBytes(float64(v.DayRX)), theme.Text)
			},
			func() {
				serialRow(c, "↑ "+locales.SerialTotal(), formatBytes(float64(v.TX)), colors.tx)
				serialRow(c, "↑ "+locales.SerialToday(), formatBytes(float64(v.DayTX)), theme.Text)
			})

		// The errors: the line's, which the driver counts, and the app's.
		section(c, locales.SerialErrors(), func() {
			if !v.ErrsOK {
				widgets.Muted(c, locales.SerialNoCounts()).FontSize(11)
			}
		})
		serialPair(c,
			func() {
				serialCount(c, locales.SerialFrameErr(), v.FrameErr, v.ErrsOK)
				serialCount(c, locales.SerialOverrunErr(), v.OverrunErr, v.ErrsOK)
				serialCount(c, locales.SerialReadErr(), v.ReadErrs, true)
			},
			func() {
				serialCount(c, locales.SerialParityErr(), v.ParityErr, v.ErrsOK)
				serialCount(c, locales.SerialBreakErr(), v.BreakErr, v.ErrsOK)
				serialCount(c, locales.SerialDrops(), v.Drops, true)
			})

		// The buffers: the driver's two, and the queue the app writes
		// through.
		section(c, locales.SerialBuffers(), nil)
		serialPair(c,
			func() {
				serialRow(c, locales.SerialInQueue(), serialQueue(v.InQueue, v.QueuesOK), theme.Text)
			},
			func() {
				serialRow(c, locales.SerialOutQueue(), serialQueue(v.OutQueue, v.QueuesOK), theme.Text)
			})
		serialRow(c, locales.SerialPending(), fmt.Sprintf("%d · %s",
			v.PendingChunks, formatBytes(float64(v.PendingBytes))), theme.Text)
		serialRow(c, locales.SerialQueueAlert(), serialAlert(v), serialAlertColor(v))

		// The timing: how the frames came, and how long the device has been
		// quiet.
		section(c, locales.SerialTiming(), nil)
		serialPair(c,
			func() {
				serialRow(c, locales.SerialLastGap(), serialGap(v.LastGap), theme.Text)
				serialRow(c, locales.SerialMaxGap(), serialGap(v.MaxGap), theme.Text)
				serialRow(c, locales.SerialMaxFrame(), serialBytes(v.MaxFrame), theme.Text)
			},
			func() {
				serialRow(c, locales.SerialAvgGap(), serialGap(v.AvgGap), theme.Text)
				serialRow(c, locales.SerialAvgFrame(), serialBytes(v.AvgFrame), theme.Text)
			})
		serialRow(c, locales.SerialLastFrame(), serialIdle(v), serialIdleColor(v))

		// The lines: how many the device printed, the one it is still
		// printing, and the words of a crash in them.
		section(c, locales.SerialParse(), nil)
		serialPair(c,
			func() {
				serialRow(c, locales.SerialFrames(), formatCount(float64(v.Frames)), theme.Text)
				serialRow(c, locales.SerialTail(), serialBytes(v.Tail), theme.Text)
			},
			func() {
				serialRow(c, locales.SerialLines(), formatCount(float64(v.Lines)), theme.Text)
				on := v.Scan
				if ui.Checkbox(c, &on, locales.SerialScan()).FontSize(11).Changed() {
					m.scan.Store(on)
				}
			})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			for i, kw := range []string{"panic", "crash", "error"} {
				color := theme.TextMuted
				if v.Keywords[i] > 0 {
					color = assetRed
				}
				ui.Text(c, fmt.Sprintf("%s %d", kw, v.Keywords[i])).FontSize(11).
					TextColor(color).Shrink(0)
			}
		})
	})
}

// serialRow is a label and its value, the value at the end of the line.
func serialRow(c *ui.Context, label, value string, color ui.Color) {
	ui.Row(c).Gap(6).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(theme.TextMuted).Shrink(0).SingleLine()
		ui.Spacer(c)
		ui.Text(c, value).FontSize(11.5).TextColor(color).SingleLine().MinWidth(0)
	})
}

// serialPair is two columns of rows side by side, so that the narrow panel
// holds more of them.
func serialPair(c *ui.Context, left, right func()) {
	ui.Row(c).Gap(12).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(left)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(right)
	})
}

// serialPinToggle is an output pin the app raises and lowers: DTR resets
// many boards, RTS holds the line of others.
func (a *App) serialPinToggle(c *ui.Context, m *serialMeter, label string, on bool, set func(bool) error) {
	if ui.Button(c, label+" "+serialOnOff(on)).FontSize(10).Shrink(0).
		Tooltip(locales.SerialPinHint()).Clicked() {
		if err := set(!on); err != nil {
			a.notify(describeError(err))
		}
		a.redraw()
	}
}

// serialModemPin is an input pin of the line: the ones a device or a modem
// raises.
func serialModemPin(c *ui.Context, label string, on, known bool) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		if !known {
			ui.Text(c, label+" —").FontSize(11).TextColor(theme.TextMuted).SingleLine()
			return
		}
		color := theme.TextMuted
		if on {
			color = assetGreen
		}
		widgets.StatusDot(c, color)
		ui.Text(c, label+" "+serialOnOff(on)).FontSize(11).TextColor(color).SingleLine()
	})
}

// serialLineState is what the driver's error counters say of the line: a
// dash where it does not report them, the errors once it has, and OK while
// it has none.
func serialLineState(v serialView) string {
	if !v.ErrsOK {
		return "—"
	}
	if v.FrameErr+v.ParityErr+v.OverrunErr+v.BreakErr > 0 {
		return locales.SerialErrors()
	}
	return locales.SerialLineOK()
}

// serialLineStateColor is red once the line has seen an error.
func serialLineStateColor(v serialView) ui.Color {
	if v.ErrsOK && v.FrameErr+v.ParityErr+v.OverrunErr+v.BreakErr > 0 {
		return assetRed
	}
	return theme.TextMuted
}

// serialCount is an error counter: red once it has counted anything.
func serialCount(c *ui.Context, label string, n uint64, known bool) {
	if !known {
		serialRow(c, label, "—", theme.TextMuted)
		return
	}
	color := theme.Text
	if n > 0 {
		color = assetRed
	}
	serialRow(c, label, formatCount(float64(n)), color)
}

// serialStatusColor is the state's colour, as the session row shows it.
func serialStatusColor(s *Session) ui.Color {
	switch s.status {
	case statusConnected:
		return assetGreen
	case statusFailed:
		return assetRed
	}
	return assetOrange
}

// serialOnOff is a pin's level as the page names it.
func serialOnOff(on bool) string {
	if on {
		return locales.SerialHigh()
	}
	return locales.SerialLow()
}

// serialGap is a gap between frames: milliseconds under a second, seconds
// over it, and a dash before there was one.
func serialGap(d time.Duration) string {
	switch {
	case d <= 0:
		return "—"
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// serialBytes is a frame's size.
func serialBytes(n int) string {
	if n <= 0 {
		return "—"
	}
	return formatBytes(float64(n))
}

// serialQueue is what a driver's buffer holds, or a dash where the system
// does not report it.
func serialQueue(n int, known bool) string {
	if !known {
		return "—"
	}
	return formatBytes(float64(n))
}

// serialAlert says whether the buffers are backing up: the driver's read
// queue full, writes queueing because the device is not taking them, or a
// write already dropped.
func serialAlert(v serialView) string {
	if v.InQueue >= serialInQueueWarn || v.PendingBytes >= serialPendingWarn || v.Drops > 0 {
		return locales.SerialQueueBacklog()
	}
	return locales.SerialQueueOK()
}

// serialAlertColor is the alert's colour: red while it holds.
func serialAlertColor(v serialView) ui.Color {
	if v.InQueue >= serialInQueueWarn || v.PendingBytes >= serialPendingWarn || v.Drops > 0 {
		return assetRed
	}
	return theme.TextMuted
}

// serialIdle is how long the device has been quiet, and whether that is
// past the point of a stopped one.
func serialIdle(v serialView) string {
	if v.LastFrame.IsZero() {
		return "—"
	}
	d := time.Since(v.LastFrame)
	if d > serialIdleWarn {
		return fmt.Sprintf(locales.SerialQuiet(), serialGap(d))
	}
	return fmt.Sprintf(locales.SerialAgo(), serialGap(d))
}

// serialIdleColor is red once the device has stopped sending.
func serialIdleColor(v serialView) ui.Color {
	if !v.LastFrame.IsZero() && time.Since(v.LastFrame) > serialIdleWarn {
		return assetRed
	}
	return theme.TextMuted
}
