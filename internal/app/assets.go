package app

import (
	"math"
	"strconv"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The assets window: the saved connections and the sessions counted, as
// QuickTerm's dashboard does on the tablet. It is a plugin of the plugin
// center, pinned to the row of session tabs by default (plugins.go), and
// counts afresh on every frame.
//
// The counts of logins and commands are kept a day at a time in
// asset_stats.json (config.AssetStatsFile): the logins that worked and
// those that did not, the commands sent, and the risky ones.

// assetsWeeks is how many weeks the activity grid covers.
const assetsWeeks = 30

// The ranges of the window's counts, as the buttons name them.
const (
	assetRangeRun  = "run"
	assetRange3d   = "3d"
	assetRange7d   = "7d"
	assetRange15d  = "15d"
	assetRange30d  = "30d"
	assetRangeAll  = "all"
	assetRangeDays = 180
)

var assetRanges = []struct {
	id    string
	label func() string
}{
	{assetRangeRun, locales.AssetsRangeRun},
	{assetRange3d, locales.AssetsRange3d},
	{assetRange7d, locales.AssetsRange7d},
	{assetRange15d, locales.AssetsRange15d},
	{assetRange30d, locales.AssetsRange30d},
	{assetRangeAll, locales.AssetsRangeAll},
}

// assetTotals are a range's counts.
type assetTotals struct {
	login, fail, cmd, risk int
}

// assetWindow is everything the window draws, counted at the frame.
type assetWindow struct {
	assets int
	online int
	failed int
	closed int

	never  int
	recent int

	authPassword int
	authKey      int
	authAgent    int

	available   int
	unknown     int
	unavailable int

	assetTotals

	// heat is the commands a day, the oldest week first, Monday first
	// within a week.
	heat    [assetsWeeks][7]int
	heatMax int
}

// assetWindowAt counts the connections, the sessions and the ranges at now.
func (a *App) assetWindowAt(now time.Time) assetWindow {
	var st assetWindow
	today := startOfDay(now)
	weekStart := startOfWeek(today)
	for _, h := range a.hosts.Live() {
		st.assets++
		switch h.Auth {
		case config.AuthKey:
			st.authKey++
		case config.AuthAgent:
			st.authAgent++
		default:
			st.authPassword++
		}
		switch {
		case h.LastConnectedAt.IsZero():
			st.never++
			st.unavailable++
		case now.Sub(h.LastConnectedAt) <= 7*24*time.Hour:
			st.recent++
			st.available++
		case now.Sub(h.LastConnectedAt) <= 30*24*time.Hour:
			st.unknown++
		default:
			st.unavailable++
		}
	}
	for _, s := range a.sessions {
		if s.Host == nil {
			continue
		}
		switch s.status {
		case statusConnected:
			st.online++
		case statusFailed:
			st.failed++
		default:
			st.closed++
		}
	}
	st.assetTotals = a.assetTotalsAt(now, a.assetRange)
	st.heat = a.assetHeat(today, weekStart)
	for _, week := range st.heat {
		for _, n := range week {
			if n > st.heatMax {
				st.heatMax = n
			}
		}
	}
	return st
}

// assetTotalsAt adds the days a range covers, today's included.
func (a *App) assetTotalsAt(now time.Time, rng string) assetTotals {
	if rng == assetRangeRun {
		return a.assetRun
	}
	days := 0
	switch rng {
	case assetRange3d:
		days = 3
	case assetRange7d:
		days = 7
	case assetRange15d:
		days = 15
	case assetRange30d:
		days = 30
	}
	from := ""
	if days > 0 {
		from = config.DayKey(now.AddDate(0, 0, -(days - 1)))
	}
	var t assetTotals
	for _, d := range a.assetDays {
		if from != "" && d.Day < from {
			continue
		}
		t.login += d.Login
		t.fail += d.LoginFail
		t.cmd += d.Cmd
		t.risk += d.Risk
	}
	return t
}

// assetHeat puts the commands of the days kept on the grid: the last week
// is this one, and a row is a day from Monday.
func (a *App) assetHeat(today, weekStart time.Time) [assetsWeeks][7]int {
	var heat [assetsWeeks][7]int
	for _, d := range a.assetDays {
		day, err := time.ParseInLocation("2006-01-02", d.Day, today.Location())
		if err != nil || day.After(today) || d.Cmd == 0 {
			continue
		}
		weeksAgo := int(weekStart.Sub(startOfWeek(day)).Hours() / (24 * 7))
		if weeksAgo < 0 || weeksAgo >= assetsWeeks {
			continue
		}
		heat[assetsWeeks-1-weeksAgo][(int(day.Weekday())+6)%7] += d.Cmd
	}
	return heat
}

// startOfDay is t at midnight, in its own zone.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// startOfWeek is the Monday of t's week, at midnight.
func startOfWeek(t time.Time) time.Time {
	d := startOfDay(t)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// assetAdd counts what happened now, and keeps the day.
func (a *App) assetAdd(login, fail, cmd, risk int) {
	a.assetRun.login += login
	a.assetRun.fail += fail
	a.assetRun.cmd += cmd
	a.assetRun.risk += risk
	key := config.DayKey(time.Now())
	last := len(a.assetDays) - 1
	if last < 0 || a.assetDays[last].Day != key {
		a.assetDays = append(a.assetDays, config.AssetDay{Day: key})
		last++
	}
	a.assetDays[last].Login += login
	a.assetDays[last].LoginFail += fail
	a.assetDays[last].Cmd += cmd
	a.assetDays[last].Risk += risk
	if len(a.assetDays) > assetRangeDays {
		a.assetDays = a.assetDays[len(a.assetDays)-assetRangeDays:]
	}
	_ = config.SaveAssetStats(a.assetDays) // the next frame counts them again
}

// assetLogin counts a login that worked, or one that did not.
func (a *App) assetLogin(ok bool) {
	if ok {
		a.assetAdd(1, 0, 0, 0)
		return
	}
	a.assetAdd(0, 1, 0, 0)
}

// assetCommand counts a command sent whole, from the command bar or a quick
// command.
func (a *App) assetCommand(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	risk := 0
	if a.risky(line) { // the danger rules decide it (danger.go)
		risk = 1
	}
	a.assetAdd(0, 0, 1, risk)
}

// assetKey follows the keys typed in s's terminal, so that Enter counts the
// command, and Ctrl+C, Ctrl+U and Backspace empty it.
func (a *App) assetKey(s *Session, mods ui.Modifiers, key ui.Key) {
	switch {
	case mods == 0 && key == ui.KeyEnter:
		line := strings.TrimSpace(s.cmdLine)
		s.cmdLine, s.cmdEdited = "", false
		if line != "" {
			a.recordCommand(s, line)
			a.assetCommand(line)
		}
	case mods == ui.Ctrl && (key == ui.KeyC || key == ui.KeyU):
		s.cmdLine, s.cmdEdited = "", false
	case mods == 0 && key == ui.KeyBackspace:
		if n := len(s.cmdLine); n > 0 {
			s.cmdLine = s.cmdLine[:n-1]
		}
	case movingKey(key):
		// The line went through the arrows: what is tracked from here on is
		// no longer what the shell has (dangerguard.go).
		s.cmdEdited = true
	default:
		if mods == 0 {
			if ch, ok := keyChar(key); ok && len(s.cmdLine) < 512 {
				s.cmdLine += ch
			}
		}
	}
}

// keyChar is the character a key types without a modifier, as the app maps
// the keys it needs for the command count.
func keyChar(key ui.Key) (string, bool) {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('a' + key - ui.KeyA)), true
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + key - ui.Key0)), true
	}
	switch key {
	case ui.KeySpace:
		return " ", true
	case ui.KeyMinus:
		return "-", true
	case ui.KeyEqual:
		return "=", true
	case ui.KeyPeriod:
		return ".", true
	case ui.KeyComma:
		return ",", true
	case ui.KeySlash:
		return "/", true
	case ui.KeySemicolon:
		return ";", true
	case ui.KeyQuote:
		return "'", true
	}
	return "", false
}

// movingKey tells whether a key moves about the line instead of typing into
// it, which leaves the tracked line unlike the shell's.
func movingKey(key ui.Key) bool {
	switch key {
	case ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd, ui.KeyDelete:
		return true
	}
	return false
}

// resetAssetStats empties the counts, the days kept and this run's.
func (a *App) resetAssetStats() {
	a.assetRun = assetTotals{}
	a.assetDays = nil
	_ = config.SaveAssetStats(nil)
}

// assetsWindow is the assets window's spec: it opens over the main window
// as the options do.
var assetsWindow = WindowSpec{
	Kind:      WindowAssets,
	Title:     locales.AssetsTitle,
	Width:     1000,
	Height:    740,
	MinWidth:  820,
	MinHeight: 520,
}

// openAssets opens the assets window, or brings it to the front.
func (a *App) openAssets() { a.openWindow(assetsWindow, a.assetsView) }

// assetsView is the window: the ranges, the counts, the cards and the grid.
func (a *App) assetsView(c *ui.Context) {
	st := a.assetWindowAt(time.Now())
	ui.Column(c).Fill().Padding(12, 14, 12, 14).Gap(10).Children(func() {
		a.assetsHeader(c)
		a.assetCounts(c, st)
		a.assetCards(c, st)
		assetHeatCard(c, st)
	})
}

// assetsHeader is the title, the ranges and reset; the window's own title
// bar closes it.
func (a *App) assetsHeader(c *ui.Context) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.AssetsTitle()).FontSize(18).FontWeight(700)
		ui.Spacer(c)
		ui.Text(c, locales.AssetsRange()).FontSize(11).TextColor(theme.TextMuted)
		for _, r := range assetRanges {
			id := r.id
			if id == a.assetRange {
				ui.PrimaryButton(c, r.label())
				continue
			}
			if ui.Button(c, r.label()).Clicked() {
				a.assetRange = id
			}
		}
		if ui.Button(c, locales.AssetsReset()).Clicked() {
			a.resetAssetStats()
		}
	})
}

// assetCounts is the row of six counts: the connections and the sessions,
// and the range's logins and commands.
func (a *App) assetCounts(c *ui.Context, st assetWindow) {
	items := []struct {
		label string
		value int
	}{
		{locales.AssetsTotal(), st.assets},
		{locales.AssetsOnline(), st.online},
		{locales.AssetsRecordings(), 0},
		{locales.AssetsCommands(), st.cmd},
		{locales.AssetsLoginOK(), st.login},
		{locales.AssetsLoginFail(), st.fail},
	}
	ui.Column(c).Gap(8).Children(func() {
		for i := 0; i < len(items); i += 3 {
			row := items[i:min(i+3, len(items))]
			ui.Row(c).Gap(8).Children(func() {
				for _, it := range row {
					ui.Column(c).Basis(0).Grow(1).Shrink(1).MinWidth(0).Gap(2).
						Padding(10, 12, 10, 12).Radius(8).Border(1, theme.Border).Children(func() {
						ui.Text(c, strconv.Itoa(it.value)).FontSize(19).FontWeight(700)
						ui.Text(c, it.label).FontSize(11).TextColor(theme.TextMuted).SingleLine().MinWidth(0)
					})
				}
			})
		}
	})
}

// assetSlice is a part of a ring.
type assetSlice struct {
	n     int
	color ui.Color
	label string
}

// assetCards is the two rows of three cards: the protocols, the health and
// the sessions, then the risky commands, the logins and the scale.
func (a *App) assetCards(c *ui.Context, st assetWindow) {
	ui.Row(c).Gap(8).Children(func() {
		assetRing(c, locales.AssetsProtocols(), []assetSlice{
			{n: st.assets, color: assetPurple, label: locales.AssetsSsh()},
		})
		assetBarCard(c, locales.AssetsHealth(), []assetBar{
			{label: locales.AssetsAvailable(), n: st.available, color: assetGreen},
			{label: locales.AssetsUnavailable(), n: st.unavailable, color: assetRed},
			{label: locales.AssetsUnknown(), n: st.unknown, color: assetOrange},
		})
		assetRing(c, locales.AssetsSessions(), []assetSlice{
			{n: st.online, color: assetGreen, label: locales.AssetsOnline()},
			{n: st.closed, color: assetBlue, label: locales.AssetsOffline()},
			{n: st.failed, color: assetRed, label: locales.StatusFailed()},
		})
	})
	ui.Row(c).Gap(8).Children(func() {
		assetBarCard(c, locales.AssetsCmdRisk(), []assetBar{
			{label: locales.AssetsNormal(), n: st.cmd - st.risk, color: assetGreen},
			{label: locales.AssetsRisky(), n: st.risk, color: assetRed},
		})
		assetRing(c, locales.AssetsLogins(), []assetSlice{
			{n: st.login, color: assetGreen, label: locales.AssetsSuccess()},
			{n: st.fail, color: assetRed, label: locales.AssetsFailed()},
		})
		assetBarCard(c, locales.AssetsScale(), []assetBar{
			{label: locales.AssetsCommands(), n: st.cmd, color: assetOrange},
			{label: locales.AssetsRecordings(), n: 0, color: assetPurple},
			{label: locales.AssetsOnline(), n: st.online, color: assetCyan},
			{label: locales.AssetsTotal(), n: st.assets, color: assetBlue},
		})
	})
}

// assetCardFrame is the frame of a card, with its title at the top.
func assetCardFrame(c *ui.Context, title string) ui.Element {
	e := ui.Column(c).Basis(0).Grow(1).Shrink(1).MinWidth(0).Gap(8).
		Padding(10, 12, 10, 12).Radius(8).Border(1, theme.Border)
	e.Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(600)
	})
	return e
}

// assetRing is a card with a ring and its legend.
func assetRing(c *ui.Context, title string, slices []assetSlice) {
	total := 0
	for _, s := range slices {
		total += s.n
	}
	assetCardFrame(c, title).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(92, 92).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
				if total > 0 {
					drawRing(p, r, slices, total)
				}
			})
			ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
				if total == 0 {
					ui.Text(c, locales.AssetsEmpty()).FontSize(11).TextColor(theme.TextMuted)
					return
				}
				for _, s := range slices {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(8, 8).Radius(4).Background(s.color).Shrink(0)
						ui.Text(c, s.label).FontSize(11).SingleLine().MinWidth(0).Grow(1)
						ui.Text(c, strconv.Itoa(s.n)).FontSize(11).TextColor(theme.TextMuted)
					})
				}
			})
		})
	})
}

// assetBarCard is a card of bars, wide in proportion to the largest.
func assetBarCard(c *ui.Context, title string, bars []assetBar) {
	maxN := 0
	for _, b := range bars {
		if b.n > maxN {
			maxN = b.n
		}
	}
	assetCardFrame(c, title).Children(func() {
		if maxN == 0 {
			ui.Text(c, locales.AssetsEmpty()).FontSize(11).TextColor(theme.TextMuted)
			return
		}
		for _, b := range bars {
			bar := b
			ui.Row(c).Gap(6).Height(18).AlignItems(ui.Center).Children(func() {
				ui.Text(c, bar.label).Width(70).FontSize(11).TextColor(theme.TextMuted).SingleLine().MinWidth(0)
				ui.Box(c).Grow(1).Shrink(1).MinWidth(0).Height(7).Radius(3).Background(assetTrack).
					Draw(func(p *ui.Painter, r ui.Rect) {
						if bar.n == 0 {
							return
						}
						p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * float32(bar.n) / float32(maxN), H: r.H}, bar.color, r.H/2)
					})
				ui.Text(c, strconv.Itoa(bar.n)).Width(30).FontSize(11).TextAlign(ui.End).TextColor(theme.TextMuted)
			})
		}
	})
}

// assetBar is one line of a card: a name, its count and the color of its
// bar.
type assetBar struct {
	label string
	n     int
	color ui.Color
}

// The colors of the counts, the bars and the grid, as QuickTerm's dashboard
// has them.
var (
	assetGreen  = ui.RGBA(0, 185, 107, 1)
	assetRed    = ui.RGBA(255, 77, 79, 1)
	assetBlue   = ui.RGBA(59, 130, 246, 1)
	assetPurple = ui.RGBA(163, 113, 247, 1)
	assetCyan   = ui.RGBA(57, 197, 207, 1)
	assetOrange = ui.RGBA(250, 173, 20, 1)
	assetTrack  = ui.RGBA(128, 128, 128, 0.25)
)

// drawRing strokes the slices as one ring, from the top round clockwise.
func drawRing(p *ui.Painter, r ui.Rect, slices []assetSlice, total int) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	rad := min(r.W, r.H)/2 - 7
	start := -math.Pi / 2
	for _, s := range slices {
		if s.n == 0 {
			continue
		}
		sweep := 2 * math.Pi * float64(s.n) / float64(total)
		steps := max(int(sweep/0.05), 3)
		var path ui.Path
		for i := 0; i <= steps; i++ {
			a := start + sweep*float64(i)/float64(steps)
			x, y := cx+rad*float32(math.Cos(a)), cy+rad*float32(math.Sin(a))
			if i == 0 {
				path.MoveTo(x, y)
			} else {
				path.LineTo(x, y)
			}
		}
		p.StrokePath(&path, 14, s.color)
		start += sweep
	}
}

// assetHeatCard is the activity card: a column a week, the last one this
// week, a row a day from Monday, a cell darker with more commands that day.
// The cells take the width of the card, taller with it up to the square they
// are at most.
func assetHeatCard(c *ui.Context, st assetWindow) {
	assetCardFrame(c, locales.AssetsActivity()).Children(func() {
		ui.Box(c).FillWidth().Grow(1).MinHeight(96).Draw(func(p *ui.Painter, r ui.Rect) {
			w, h := r.W/assetsWeeks, min(r.H/7, r.W/assetsWeeks) // square at most
			oy := r.Y + (r.H-h*7)/2
			for col := 0; col < assetsWeeks; col++ {
				for row := 0; row < 7; row++ {
					p.Fill(ui.Rect{
						X: r.X + float32(col)*w + 0.5,
						Y: oy + float32(row)*h + 0.5,
						W: w - 1,
						H: h - 1,
					}, assetHeatColor(st.heat[col][row], st.heatMax), 1.5)
				}
			}
		})
		ui.Text(c, locales.AssetsHeatNote()).FontSize(10).TextColor(theme.TextMuted)
	})
}

// assetHeatColor is a cell's color: no command is the track, and the rest
// go from light to solid blue in four steps up to the busiest day.
func assetHeatColor(n, maxN int) ui.Color {
	if n <= 0 || maxN <= 0 {
		return assetTrack
	}
	level := (n*4 + maxN - 1) / maxN // 1..4, rounded up
	switch level {
	case 1:
		return ui.RGBA(59, 130, 246, 0.25)
	case 2:
		return ui.RGBA(59, 130, 246, 0.45)
	case 3:
		return ui.RGBA(59, 130, 246, 0.7)
	default:
		return assetBlue
	}
}
