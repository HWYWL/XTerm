package app

import (
	"testing"
	"time"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// The daily report counts today from the days the assets window keeps, and
// shows the week's trend under it.
func TestOpsReport(t *testing.T) {
	a, p := testApp(t)
	now := time.Now()
	a.assetDays = []config.AssetDay{
		{Day: config.DayKey(now), Login: 3, LoginFail: 1, Cmd: 7, Risk: 2},
		{Day: config.DayKey(now.AddDate(0, 0, -1)), Login: 1, Cmd: 4},
	}

	a.openPlugin(pluginOps)
	w := p.show(t, WindowOpsReport)
	w.Frame()
	for _, s := range []string{
		locales.OpsNote(), locales.OpsLogin(), locales.OpsFail(), locales.OpsCmd(),
		locales.OpsRisk(), locales.OpsWeekCmd(), locales.OpsWeekRisk(),
	} {
		if !w.HasText(s) {
			t.Errorf("no %q; texts %q", s, w.Texts())
		}
	}
	// Today's commands and risky ones, and yesterday's label in the bars.
	for _, s := range []string{"7", "2", config.DayKey(now)[5:]} {
		if !w.HasText(s) {
			t.Errorf("no %q; texts %q", s, w.Texts())
		}
	}
}
