package app

import (
	"fmt"
	"strconv"

	"finallyshell/internal/icons"
	"finallyshell/internal/monitor"
	"finallyshell/internal/redis"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// redisPanel is the System page of a Redis tab — the side panel's own for
// the protocol, as the SSH one is —: what INFO says of the server, read
// every redisInfoPoll (redisinfo.go). The top sums it up as the SSH panel
// sums a server up, with its bars; below, every section INFO sends, the
// chosen one's fields as the key and value they are.
func (a *App) redisPanel(c *ui.Context, s *Session) {
	ui.Column(c).FillWidth().Padding(10, 10, 8, 10).Gap(6).Children(func() {
		addr := ""
		if s != nil && s.Host != nil {
			addr = s.redisOpts.Addr
			if addr == "" || s.redisOpts.Cluster {
				addr = s.Host.Target()
			}
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "IP "+addr).SingleLine().MinWidth(0)
			ui.Spacer(c)
			cp := ui.Box(c).Size(16, 16).Center().Radius(4).Shrink(0).
				Label(locales.Copy()).Tooltip(locales.Copy()).Cursor(ui.CursorPointer)
			if cp.Hovered() {
				cp.Background(theme.Text.Alpha(0.07))
			}
			cp.Children(func() {
				ui.Icon(c, icons.Get("copy")).FontSize(12).TextColor(theme.Accent)
			})
			if cp.Clicked() {
				a.copyToClipboard(c, addr)
			}
		})
		switch {
		case s == nil || s.status != statusConnected:
			widgets.Muted(c, s.statusText())
			return
		case s.redisInfoErr != nil:
			widgets.Muted(c, fmt.Sprintf(locales.RedisInfoFailed(), describeError(s.redisInfoErr)))
			return
		case s.redisInfoAt.IsZero():
			widgets.Muted(c, locales.ReadingServerInfo())
			return
		}
		in := s.redisInfo
		// What the server is: master or replica, its Redis and its system
		// version, the clients connected, how long it has run.
		line := in.Value("Replication", "role")
		if v := in.Value("Server", "redis_version"); v != "" {
			line = joinDot(line, "Redis "+v)
		}
		if os := in.Value("Server", "os"); os != "" {
			line = joinDot(line, os)
		}
		line = joinDot(line, fmt.Sprintf(locales.RedisClientsNow(), in.Int("Clients", "connected_clients")))
		if days := in.Int("Server", "uptime_in_days"); days >= 0 {
			line = joinDot(line, locales.UptimeDays(int(days)))
		}
		widgets.Muted(c, line).FontSize(11.5).Wrap()

		redisBars(c, in)
		redisRings(c, in)
		redisOpsCard(c, s)
		redisNetCard(c, s)
		redisKeysCard(c, in)

		if len(in.Sections) == 0 {
			return
		}
		// The sections' fields are more than a narrow panel shows: the
		// button opens them, with what each field means (redisdialog.go).
		a.redisSection = a.redisSectionShown(in)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			widgets.Muted(c, locales.RedisSection()).FontSize(11.5)
			if ui.Button(c, a.redisSection).Grow(1).Tooltip(locales.RedisSectionsHint()).Clicked() {
				a.openRedisSections(s, a.redisSection)
			}
		})
	})
}

// joinDot joins the parts that are not empty with a dot.
func joinDot(parts ...string) string {
	line := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if line != "" {
			line += " · "
		}
		line += p
	}
	return line
}

// redisBars are the panel's bars, as the SSH panel's resources: the memory
// the server holds of its limit, the clients of those it takes, and how
// many of the lookups it answered from memory.
func redisBars(c *ui.Context, in redis.Info) {
	used := uint64(max(in.Int("Memory", "used_memory"), 0))
	if limit := uint64(max(in.Int("Memory", "maxmemory"), 0)); limit > 0 {
		widgets.UsageBar(c, locales.Memory(), ratio(used, limit), usage(used, limit), colors.mem)
	} else {
		widgets.UsageBar(c, locales.Memory(), 0, formatBytes(float64(used))+" / "+locales.RedisNoLimit(), colors.mem)
	}
	clients := uint64(max(in.Int("Clients", "connected_clients"), 0))
	maxClients := uint64(max(in.Int("Clients", "maxclients"), 0))
	widgets.UsageBar(c, locales.RedisClients(), ratio(clients, maxClients), usage(clients, maxClients), colors.cpu)

	hits, misses := in.Int("Stats", "keyspace_hits"), in.Int("Stats", "keyspace_misses")
	rate, text := 0.0, "-"
	if hits+misses > 0 {
		rate = float64(hits) / float64(hits+misses)
		text = percentText(rate)
	}
	widgets.UsageBar(c, locales.RedisHitRate(), rate, text, colors.disk)

	ui.Row(c).Gap(10).Margin(2, 0, 0, 0).Children(func() {
		widgets.Muted(c, fmt.Sprintf(locales.RedisOps(), in.Int("Stats", "instantaneous_ops_per_sec"))).FontSize(11)
		widgets.Muted(c, fmt.Sprintf(locales.RedisKeysSum(), in.Keys())).FontSize(11)
	})
}

// redisRings are the rings under the bars, as the SSH page's: the memory
// the server holds of its limit, the clients of those it takes, and the
// share of the lookups it answered itself.
func redisRings(c *ui.Context, in redis.Info) {
	mem := 0.0
	if limit := uint64(max(in.Int("Memory", "maxmemory"), 0)); limit > 0 {
		mem = ratio(uint64(max(in.Int("Memory", "used_memory"), 0)), limit)
	}
	clients := ratio(uint64(max(in.Int("Clients", "connected_clients"), 0)),
		uint64(max(in.Int("Clients", "maxclients"), 0)))
	hits, misses := in.Int("Stats", "keyspace_hits"), in.Int("Stats", "keyspace_misses")
	hit := 0.0
	if hits+misses > 0 {
		hit = float64(hits) / float64(hits+misses)
	}
	ui.Row(c).Gap(4).Margin(0, 2, 0, 0).Children(func() {
		widgets.Donut(c, locales.Memory(), mem, percentText(mem), colors.memRing)
		widgets.Donut(c, locales.RedisClients(), clients, percentText(clients), colors.cpuRing)
		widgets.Donut(c, locales.RedisHitRate(), hit, percentText(hit), colors.diskRing)
	})
}

// redisOpsCard is the commands the server runs, as the curve of the
// samples the panel read, with the latest and the largest.
func redisOpsCard(c *ui.Context, s *Session) {
	section(c, locales.RedisOpsTitle(), func() {
		widgets.Muted(c, graphWindow()).FontSize(11)
	})
	ui.Row(c).Gap(10).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		widgets.Muted(c, fmt.Sprintf(locales.RedisOps(),
			s.redisInfo.Int("Stats", "instantaneous_ops_per_sec"))).FontSize(12)
		ui.Spacer(c)
		if peak := histPeak(s.redisOpsHist); peak > 0 {
			widgets.Muted(c, fmt.Sprintf(locales.NetPeak(), formatCount(peak))).FontSize(11)
		}
	})
	widgets.Curve(c, monitor.HistoryLen, [][]float64{s.redisOpsHist}, []ui.Color{colors.cpuRing}, 46)
}

// redisNetCard is the traffic the server read and wrote, from the totals
// INFO counts, which the panel turns into the rates of the curve.
func redisNetCard(c *ui.Context, s *Session) {
	last := func(h []float64) float64 {
		if len(h) == 0 {
			return 0
		}
		return h[len(h)-1]
	}
	section(c, locales.RedisNetTitle(), func() {
		widgets.Muted(c, graphWindow()).FontSize(11)
	})
	ui.Row(c).Gap(10).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "↓ "+formatRate(last(s.redisInHist))).TextColor(colors.rx).FontSize(12).Shrink(0)
		ui.Text(c, "↑ "+formatRate(last(s.redisOutHist))).TextColor(colors.tx).FontSize(12).Shrink(0)
		ui.Spacer(c)
		if peak := histPeak(s.redisInHist, s.redisOutHist); peak > 0 {
			widgets.Muted(c, fmt.Sprintf(locales.NetPeak(), formatRate(peak))).FontSize(11)
		}
	})
	widgets.Curve(c, monitor.HistoryLen, [][]float64{s.redisInHist, s.redisOutHist}, []ui.Color{rxChart, txChart}, 46)
}

// redisKeysCard is the keys of each database, a bar for each, the fullest
// taking all of it.
func redisKeysCard(c *ui.Context, in redis.Info) {
	dbs := in.Databases()
	if len(dbs) == 0 {
		return
	}
	var most int64
	for _, db := range dbs {
		most = max(most, db.Keys)
	}
	section(c, locales.RedisKeysTitle(), func() {
		widgets.Muted(c, fmt.Sprintf(locales.RedisKeysSum(), in.Keys())).FontSize(11)
	})
	for _, db := range dbs {
		widgets.UsageBar(c, db.Name, ratio(uint64(db.Keys), uint64(most)),
			fmt.Sprintf(locales.RedisKeyRow(), db.Keys, db.Expires), colors.disk)
	}
}

// formatCount is n in thousands, millions or billions, for the largest of a
// curve of counts.
func formatCount(n float64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1fB", n/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", n/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.1fK", n/1e3)
	}
	return strconv.FormatInt(int64(n+0.5), 10)
}
