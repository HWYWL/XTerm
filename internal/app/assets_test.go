package app

import (
	"testing"
	"time"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The assets window counts the saved connections and the sessions: how many
// there are, how they log in, when each was last connected, and which are in
// use now.
func TestAssetStats(t *testing.T) {
	a, _ := testApp(t)
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local) // a Tuesday
	add := func(id string, auth config.AuthKind, at time.Time) *config.Host {
		h := &config.Host{ID: id, Name: id, Addr: "10.0.0.1", User: "root", Auth: auth, LastConnectedAt: at}
		a.hosts.Add(h)
		return h
	}
	key := add("key", config.AuthKey, now.Add(-2*time.Hour))
	add("password", config.AuthPassword, now.AddDate(0, 0, -1))
	add("agent", config.AuthAgent, now.AddDate(0, 0, -8))
	add("old", config.AuthKey, now.AddDate(0, 0, -200))
	add("never", config.AuthPassword, time.Time{})

	// Two sessions in use, one failed, one closed, and two rows with no
	// host: a new tab and a local terminal.
	a.sessions = []*Session{
		{Host: key, status: statusConnected},
		{Host: key, status: statusConnected},
		{Host: key, status: statusFailed},
		{Host: key, status: statusClosed},
		{status: statusConnected},
		{local: true, status: statusConnected},
	}
	// The commands of the days kept, this week's and the week before's.
	a.assetDays = []config.AssetDay{
		{Day: "2026-11-16", Cmd: 2},
		{Day: "2026-11-22", Login: 2, LoginFail: 1, Cmd: 3, Risk: 1},
		{Day: "2026-11-23", Cmd: 1},
		{Day: "2026-11-24", Cmd: 4},
	}

	st := a.assetWindowAt(now)
	if st.assets != 5 || st.online != 2 || st.failed != 1 || st.closed != 1 {
		t.Errorf("counts %+v", st)
	}
	if st.recent != 2 || st.never != 1 {
		t.Errorf("recent %d never %d", st.recent, st.never)
	}
	if st.authPassword != 2 || st.authKey != 2 || st.authAgent != 1 {
		t.Errorf("auth %d %d %d", st.authPassword, st.authKey, st.authAgent)
	}
	if st.available != 2 || st.unknown != 1 || st.unavailable != 2 {
		t.Errorf("health %d %d %d", st.available, st.unknown, st.unavailable)
	}
	// This week's column is the last: Tuesday and Monday there, the week
	// before for the Sunday.
	if st.heat[assetsWeeks-1][1] != 4 || st.heat[assetsWeeks-1][0] != 1 {
		t.Errorf("this week %v", st.heat[assetsWeeks-1])
	}
	if st.heat[assetsWeeks-2][6] != 3 || st.heat[assetsWeeks-2][0] != 2 {
		t.Errorf("last week %v", st.heat[assetsWeeks-2])
	}
	if st.heatMax != 4 {
		t.Errorf("max %d", st.heatMax)
	}
	// A week's counts: the Sunday, Monday and today.
	if st.cmd != 8 || st.login != 2 || st.fail != 1 || st.risk != 1 {
		t.Errorf("a week: %+v", st.assetTotals)
	}
	a.assetRange = assetRangeAll
	if got := a.assetTotalsAt(now, a.assetRange); got.cmd != 10 {
		t.Errorf("all: %+v", got)
	}
	// This run's, which reset empties.
	a.assetRun = assetTotals{login: 1, cmd: 1}
	if got := a.assetTotalsAt(now, assetRangeRun); got != a.assetRun {
		t.Errorf("this run: %+v", got)
	}
}

// The assets window counts a command sent whole, and the risky ones, and
// keeps the day: a reset empties the counts and the file.
func TestAssetCommands(t *testing.T) {
	a, _ := testApp(t)
	a.assetCommand("rm -rf /")
	a.assetCommand("ls")
	if a.assetRun.cmd != 2 || a.assetRun.risk != 1 {
		t.Errorf("counts %+v", a.assetRun)
	}
	// A line typed in the terminal counts on Enter, as it runs.
	s := &Session{}
	for _, k := range []ui.Key{ui.KeyR, ui.KeyM, ui.KeySpace, ui.KeyMinus, ui.KeyR, ui.KeyF,
		ui.KeySpace, ui.KeySlash} {
		a.assetKey(s, 0, k)
	}
	a.assetKey(s, 0, ui.KeyEnter)
	if s.cmdLine != "" || a.assetRun.cmd != 3 || a.assetRun.risk != 2 {
		t.Errorf("typed %q: %+v", s.cmdLine, a.assetRun)
	}
	if days := config.LoadAssetStats(); len(days) != 1 || days[0].Day != config.DayKey(time.Now()) || days[0].Cmd != 3 {
		t.Errorf("kept %+v", days)
	}

	a.resetAssetStats()
	if a.assetRun != (assetTotals{}) || len(a.assetDays) != 0 || len(config.LoadAssetStats()) != 0 {
		t.Errorf("after reset: %+v, %+v", a.assetRun, a.assetDays)
	}
}

// The icon at the right end of the row of session tabs opens the assets
// window, which shows the counts, the cards and the grid, and chooses the
// range it counts.
func TestAssetsWindow(t *testing.T) {
	a, p := testApp(t)
	now := time.Now()
	for _, h := range []*config.Host{
		{ID: "a", Name: "web-1", Addr: "45.56.78.90", User: "root", Auth: config.AuthKey, LastConnectedAt: now},
		{ID: "b", Name: "db-1", Addr: "45.56.78.91", User: "root", Auth: config.AuthPassword},
	} {
		a.hosts.Add(h)
	}
	a.assetDays = []config.AssetDay{{Day: config.DayKey(now), Login: 1, Cmd: 3}}

	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	if err := tt.Click(locales.PluginAssetsName()); err != nil {
		t.Fatalf("no assets icon on the row: %v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !a.windowOpen(WindowAssets) {
		t.Fatalf("no assets window; windows %v", p.windows)
	}

	w := p.show(t, WindowAssets)
	w.Frame()
	for _, s := range []string{
		locales.AssetsTotal(), locales.AssetsOnline(), locales.AssetsProtocols(), locales.AssetsHealth(),
		locales.AssetsSessions(), locales.AssetsActivity(), locales.AssetsHeatNote(), locales.AssetsEmpty(),
	} {
		if !w.HasText(s) {
			t.Errorf("no %q; texts %q", s, w.Texts())
		}
	}
	if !w.HasText(locales.AssetsSsh()) || !w.HasText("3") {
		t.Errorf("the counts of the cards; texts %q", w.Texts())
	}

	// The range buttons count it again, and Escape closes it, as the title
	// bar's close button does.
	if err := w.Click(locales.AssetsRange30d()); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	if a.assetRange != assetRange30d {
		t.Errorf("range %q", a.assetRange)
	}
	w.Key(0, ui.KeyEscape)
	if a.windowOpen(WindowAssets) {
		t.Error("Escape left the window open")
	}
}
