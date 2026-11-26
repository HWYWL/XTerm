package app

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// objectPanel is the System page of an object storage tab: whether its
// endpoint answers, how fast, what the tab's calls carried and cost, and
// what the bucket holds (objectmetrics.go).
func (a *App) objectPanel(c *ui.Context, s *Session) {
	if s.status != statusConnected || s.object == nil {
		widgets.Muted(c, s.statusText()).Wrap()
		if s.err != nil {
			ui.Text(c, describeError(s.err)).FontSize(11.5).TextColor(theme.Danger).Wrap()
		}
		return
	}
	m := s.objectMeter.snapshot()
	a.objectStatusHeader(c, s, m)
	objectRings(c, m)
	objectTraffic(c, m)
	objectRequests(c, m)
	objectLatency(c, m)
	objectCodes(c, m)
}

// objectHeader is where the storage is, and whether it answers.
func (a *App) objectStatusHeader(c *ui.Context, s *Session, m objectSnapshot) {
	h := s.Host
	where := ""
	if h.Object != nil && h.Object.Bucket != "" {
		where = h.Object.Bucket
	}
	if h.Object != nil && h.Object.LocalPath != "" {
		where = h.Object.LocalPath
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, where).FontSize(12).Bold().SingleLine().Tooltip(where).MinWidth(0).Shrink(1)
		ui.Spacer(c)
		if widgets.IconButton(c, "copy", locales.CopyPath()).Clicked() {
			c.WriteClipboard(where)
		}
		if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Clicked() && s.ctx != nil {
			a.probeObject(s, s.ctx, s.object)
		}
	})
	ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(2, 0, 0, 0).Children(func() {
		dot := theme.TextMuted
		state := locales.ObjectOffline()
		switch m.probeState {
		case "online":
			dot, state = theme.Success, locales.ObjectOnline()
		case "denied":
			dot, state = theme.Warning, locales.ObjectDenied()
		case "timeout":
			dot, state = theme.Warning, locales.ObjectTimeout()
		case "":
			state = locales.ObjectProbing()
		}
		widgets.StatusDot(c, dot)
		ui.Text(c, state).FontSize(11.5)
		if m.probeLat > 0 && m.probeState != "timeout" {
			widgets.Muted(c, fmt.Sprintf(locales.ObjectProbeLatency(), m.probeLat.Round(time.Millisecond))).FontSize(11)
		}
		ui.Spacer(c)
		if !m.probeAt.IsZero() {
			widgets.Muted(c, m.probeAt.Format("15:04:05")).FontSize(10.5)
		}
	})
	if m.probeErr != nil && m.probeState != "online" && m.probeState != "denied" {
		widgets.Muted(c, describeError(m.probeErr)).FontSize(10.5).SingleLine().
			Tooltip(describeError(m.probeErr))
	}
	widgets.Muted(c, s.object.Kind()).FontSize(11)
}

// objectRings are the share of the calls that worked, and the samples the
// page has seen.
func objectRings(c *ui.Context, m objectSnapshot) {
	total := m.ok + m.failed
	rate := 0.0
	if total > 0 {
		rate = float64(m.ok) / float64(total)
	}
	text := "-"
	if total > 0 {
		text = percentText(rate)
	}
	ui.Row(c).Gap(6).Margin(2, 2, 0, 0).AlignItems(ui.Center).Children(func() {
		widgets.Donut(c, locales.ObjectSuccess(), rate, text, colors.cpuRing)
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Row(c).Children(func() {
				ui.Text(c, locales.ObjectOkCalls()).FontSize(11)
				ui.Spacer(c)
				ui.Text(c, formatCount(float64(m.ok))).FontSize(11).TextColor(theme.Success)
			})
			ui.Row(c).Children(func() {
				ui.Text(c, locales.ObjectFailed()).FontSize(11)
				ui.Spacer(c)
				ui.Text(c, fmt.Sprintf("%s · %s", formatCount(float64(m.failed)),
					fmt.Sprintf(locales.ObjectTimeoutCalls(), m.timeouts))).
					FontSize(11).TextColor(theme.Danger)
			})
		})
	})
}

// objectTraffic is what the tab read and wrote, now and today.
func objectTraffic(c *ui.Context, m objectSnapshot) {
	last := func(h []float64) float64 {
		if len(h) == 0 {
			return 0
		}
		return h[len(h)-1]
	}
	section(c, locales.ObjectTraffic(), func() {
		widgets.Muted(c, fmt.Sprintf(locales.ObjectToday(), formatSize(int64(m.todayIn)), formatSize(int64(m.todayOut)))).FontSize(10.5)
	})
	ui.Row(c).Gap(10).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "↓ "+formatRate(last(m.inRate))).TextColor(colors.rx).FontSize(12).Shrink(0)
		ui.Text(c, "↑ "+formatRate(last(m.outRate))).TextColor(colors.tx).FontSize(12).Shrink(0)
		ui.Spacer(c)
		if peak := histPeak(m.inRate, m.outRate); peak > 0 {
			widgets.Muted(c, fmt.Sprintf(locales.NetPeak(), formatRate(peak))).FontSize(11)
		}
	})
	widgets.Curve(c, monitor.HistoryLen, [][]float64{m.inRate, m.outRate}, []ui.Color{rxChart, txChart}, 46)
}

// objectRequests is the calls a second of each verb, and how many there
// were.
func objectRequests(c *ui.Context, m objectSnapshot) {
	verbs := []string{"Put", "Get", "List", "Delete"}
	var most float64
	counts := map[string]int64{}
	for _, verb := range verbs {
		counts[verb] = m.count(verb)
		if h := m.qps[verb]; len(h) > 0 && h[len(h)-1] > most {
			most = h[len(h)-1]
		}
	}
	section(c, locales.ObjectRequests(), func() {
		widgets.Muted(c, graphWindow()).FontSize(10.5)
	})
	for _, verb := range verbs {
		now := 0.0
		if h := m.qps[verb]; len(h) > 0 {
			now = h[len(h)-1]
		}
		text := fmt.Sprintf("%s/s · %s", formatQPS(now), fmt.Sprintf(locales.ObjectCalls(), counts[verb]))
		widgets.UsageBar(c, verb, ratio64(now, most), text, colors.disk)
	}
}

// objectLatency is the end-to-end time of each verb, at the fiftieth, the
// ninetieth and the ninety-ninth percentile of its calls.
func objectLatency(c *ui.Context, m objectSnapshot) {
	verbs := []string{"Put", "Get", "List", "Delete"}
	section(c, locales.ObjectLatency(), func() {
		widgets.Muted(c, graphWindow()).FontSize(10.5)
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(46, 1)
		ui.Spacer(c)
		for _, p := range []string{"P50", "P90", "P99"} {
			widgets.Muted(c, p).FontSize(10.5).Width(56).Shrink(0)
		}
	})
	for _, verb := range verbs {
		p50, p90, p99 := objectPercentiles(m.latency(verb))
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, verb).FontSize(11).Width(46).Shrink(0)
			ui.Spacer(c)
			for _, d := range []time.Duration{p50, p90, p99} {
				text := "—"
				if d > 0 {
					text = d.Round(time.Millisecond).String()
				}
				ui.Text(c, text).FontSize(11).Width(56).Shrink(0)
			}
		})
	}
}

// objectCodes counts the answers by their status, and the requests that
// never got one.
func objectCodes(c *ui.Context, m objectSnapshot) {
	codes := []int{200, 401, 403, 404, 500}
	var total int64 = m.timeouts
	shown := map[string]int64{locales.ObjectTimeout(): m.timeouts}
	for _, code := range codes {
		n := m.codes[code]
		total += n
		if n > 0 {
			shown[strconv.Itoa(code)] = n
		}
	}
	if total == 0 {
		section(c, locales.ObjectCodes(), func() {})
		widgets.Muted(c, locales.ObjectNoSamples()).FontSize(11)
		return
	}
	keys := make([]string, 0, len(shown))
	for k, n := range shown {
		if n > 0 || k == locales.ObjectTimeout() {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return shown[keys[i]] > shown[keys[j]] })
	section(c, locales.ObjectCodes(), func() {
		widgets.Muted(c, fmt.Sprintf(locales.ObjectCalls(), total)).FontSize(10.5)
	})
	for _, k := range keys {
		n := shown[k]
		color := colors.disk
		switch k {
		case "200":
			color = theme.Success
		case "401", "403":
			color = theme.Warning
		case "404", "500":
			color = theme.Danger
		case locales.ObjectTimeout():
			color = theme.Danger
		}
		widgets.UsageBar(c, k, ratio64(float64(n), float64(total)), formatCount(float64(n)), color)
	}
}

// objectPercentiles are the latencies at p50, p90 and p99, of the calls
// kept.
func objectPercentiles(lat []time.Duration) (p50, p90, p99 time.Duration) {
	if len(lat) == 0 {
		return 0, 0, 0
	}
	sorted := append([]time.Duration(nil), lat...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	at := func(p float64) time.Duration {
		i := int(float64(len(sorted)-1) * p)
		return sorted[i]
	}
	return at(0.50), at(0.90), at(0.99)
}

// ratio64 is n of most, 0 when most is 0.
func ratio64(n, most float64) float64 {
	if most <= 0 {
		return 0
	}
	return n / most
}

// formatQPS writes calls a second with one decimal when it is small.
func formatQPS(v float64) string {
	if v >= 100 {
		return strconv.FormatInt(int64(v+0.5), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}
