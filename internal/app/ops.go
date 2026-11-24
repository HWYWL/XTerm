package app

import (
	"strconv"
	"time"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The ops report window: the day counted as QuickTerm's report counts it,
// from what the assets window keeps in asset_stats.json: the logins and
// their failures, the commands and the risky ones, and the week's trend
// below them.

// opsWindow is its spec.
var opsWindow = WindowSpec{
	Kind:      WindowOpsReport,
	Title:     locales.OpsTitle,
	Width:     760,
	Height:    560,
	MinWidth:  620,
	MinHeight: 420,
}

// openOpsReport opens the report, which reads the days afresh each frame.
func (a *App) openOpsReport() { a.openWindow(opsWindow, a.opsView) }

// opsView is the window: today's four numbers, then the week's bars.
func (a *App) opsView(c *ui.Context) {
	ui.Column(c).Fill().Padding(12, 14, 12, 14).Gap(10).Children(func() {
		ui.Text(c, locales.OpsNote()).FontSize(11).TextColor(theme.TextMuted).MaxLines(2)
		today := assetDayOf(a.assetDays, config.DayKey(time.Now()))
		ui.Row(c).Gap(8).Children(func() {
			statCard(c, locales.OpsLogin(), strconv.Itoa(today.Login), "", assetGreen)
			statCard(c, locales.OpsFail(), strconv.Itoa(today.LoginFail), "", assetRed)
			statCard(c, locales.OpsCmd(), strconv.Itoa(today.Cmd), "", assetBlue)
			statCard(c, locales.OpsRisk(), strconv.Itoa(today.Risk), "", assetOrange)
		})
		// The week, the oldest day on the left.
		cmd, risk := make([]assetBar, 0, 7), make([]assetBar, 0, 7)
		now := time.Now()
		for i := 6; i >= 0; i-- {
			key := config.DayKey(now.AddDate(0, 0, -i))
			d := assetDayOf(a.assetDays, key)
			label := key[5:] // MM-DD
			cmd = append(cmd, assetBar{label: label, n: d.Cmd, color: assetBlue})
			risk = append(risk, assetBar{label: label, n: d.Risk, color: assetOrange})
		}
		ui.Row(c).Gap(8).Children(func() {
			assetBarCard(c, locales.OpsWeekCmd(), cmd)
			assetBarCard(c, locales.OpsWeekRisk(), risk)
		})
	})
}

// assetDayOf is the day key in days, empty for one not kept.
func assetDayOf(days []config.AssetDay, key string) config.AssetDay {
	for _, d := range days {
		if d.Day == key {
			return d
		}
	}
	return config.AssetDay{}
}

// statCardHeight is the height every card of a row has, with a hint or
// without: a row's cards then line up with the next row's.
const statCardHeight = 92

// statCard is one number of the reports: its label, the number large, and a
// hint under it where there is one.
func statCard(c *ui.Context, label, value, hint string, color ui.Color) {
	ui.Column(c).Basis(0).Grow(1).Shrink(1).MinWidth(0).Height(statCardHeight).Gap(4).
		Padding(10, 12, 10, 12).Radius(8).Border(1, theme.Border).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(theme.TextMuted).SingleLine()
		ui.Text(c, value).FontSize(22).FontWeight(700).TextColor(color).SingleLine().Ellipsis("…")
		if hint != "" {
			ui.Text(c, hint).FontSize(10).TextColor(theme.TextMuted).SingleLine()
		}
	})
}
