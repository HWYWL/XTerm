package app

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/monitor"
	"finallyshell/internal/redis"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// cacheInfo is what a cache says of itself, as INFO prints it.
const cacheInfo = `# Server
redis_version:7.2.4
redis_mode:standalone
os:Linux 6.1.0 x86_64
uptime_in_days:12
a_field_of_a_later_server:1

# Clients
connected_clients:7
blocked_clients:0
maxclients:10000

# Memory
used_memory:104857600
maxmemory:536870912
maxmemory_policy:noeviction

# Stats
instantaneous_ops_per_sec:412
keyspace_hits:9700
keyspace_misses:300
total_net_input_bytes:734003200
total_net_output_bytes:113246208

# Replication
role:master
connected_slaves:0

# Keyspace
db0:keys=1200,expires=12,avg_ttl=0
`

// redisPanelApp is the main window on a Redis session whose panel has
// read info, info being "" for none yet.
func redisPanelApp(t *testing.T, info string) (*App, *ui.Tester, *Session) {
	t.Helper()
	a, p := testApp(t)
	s := demoSession(a, "cache", "45.56.78.90")
	s.Host.Protocol = config.ProtocolRedis
	if info != "" {
		s.redisInfo = redis.ParseInfo(info)
		s.redisInfoAt = time.Now()
		// A minute of samples, as a connected panel would have: the
		// curves of the commands and of the traffic.
		for i := 0; i < monitor.HistoryLen; i++ {
			s.redisOpsHist = append(s.redisOpsHist, 300+120*math.Sin(float64(i)/6))
			s.redisInHist = append(s.redisInHist, 22000+9000*math.Sin(float64(i)/5+1))
			s.redisOutHist = append(s.redisOutHist, 6000+2500*math.Cos(float64(i)/7))
		}
	}
	a.active = 0
	a.setLeftPage(leftSystem)
	tt := ui.NewTester(func(c *ui.Context) { a.View(c) }, 1280, 820)
	p.tt = tt
	tt.Frame()
	return a, tt, s
}

// The System page of a Redis tab is the server's own: its bars and the
// sections of INFO, the chosen one's fields as keys and values, which the
// SSH page does not show.
func TestRedisPanelShowsInfo(t *testing.T) {
	a, tt, s := redisPanelApp(t, cacheInfo)
	for _, want := range []string{
		"IP 45.56.78.90:6379",
		// The line under it: the role, both versions, the clients and the
		// uptime.
		"master", "Redis 7.2.4", "Linux 6.1.0 x86_64", "7 clients", locales.UptimeDays(12),
		locales.Memory(), locales.RedisClients(), locales.RedisHitRate(),
		locales.RedisSection(), "Server",
		// The charts: the commands and the traffic, and a bar for the
		// database's keys.
		locales.RedisOpsTitle(), locales.RedisNetTitle(), locales.RedisKeysTitle(),
		"db0",
	} {
		if !tt.HasText(want) {
			t.Errorf("%q is not shown; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText(locales.Cpu()) || tt.HasText(locales.Swap()) {
		t.Error("the SSH page's parts are shown")
	}
	saveShot(t, tt, "redis-panel-server-en.png")

	// The section button opens the dialog: the sections to choose from,
	// and the fields with what each means.
	if err := tt.Click("Server"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.redisSectionsDlg == nil {
		t.Fatal("the button opened no dialog")
	}
	for _, want := range []string{
		locales.RedisSectionsTitle(), locales.RedisColField(), locales.RedisColValue(),
		locales.RedisColMeaning(), "redis_version", redisInfoDoc("redis_version"),
	} {
		if !tt.HasText(want) {
			t.Errorf("the dialog lacks %q; texts %q", want, tt.Texts())
		}
	}
	// A field of a newer server than the app knows says it is not
	// documented, rather than showing nothing.
	if !tt.HasText(locales.RedisNoMeaning()) {
		t.Errorf("no note for the field it does not know; texts %q", tt.Texts())
	}
	saveShot(t, tt, "redis-sections-en.png")

	// Another section shows its fields, and its meaning, and is what the
	// page shows after it.
	if err := tt.Click("Memory"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.redisSection != "Memory" || a.redisSectionsDlg.section != "Memory" {
		t.Errorf("the section %q, the dialog's %q", a.redisSection, a.redisSectionsDlg.section)
	}
	for _, want := range []string{"used_memory", redisInfoDoc("used_memory"), "maxmemory_policy"} {
		if !tt.HasText(want) {
			t.Errorf("the memory section lacks %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText("redis_version") {
		t.Error("the section before it is still shown")
	}
	saveShot(t, tt, "redis-sections-memory-en.png")
	_ = s
}

// Every field the app knows has what it means in both languages, and the
// databases and errors are known by their name.
func TestRedisInfoDoc(t *testing.T) {
	if got := redisInfoDoc("used_memory"); got == "" {
		t.Error("used_memory has no meaning")
	}
	if got := redisInfoDoc("db3"); got == "" {
		t.Error("a database has no meaning")
	}
	if got := redisInfoDoc("errorstat_WRONGTYPE"); got == "" {
		t.Error("an error has no meaning")
	}
	if got := redisInfoDoc("a_field_of_a_later_server"); got != "" {
		t.Errorf("a field it does not know: %q", got)
	}
	for field, d := range redisFieldDocs {
		if d.en == "" || d.zh == "" {
			t.Errorf("%q lacks a language: %+v", field, d)
		}
	}
	// The dialog speaks the app's language: Chinese when the app does.
	_, p := testApp(t)
	p.SetLocale("zh-Hans")
	defer p.SetLocale("en")
	if got := redisInfoDoc("used_memory"); got != redisFieldDocs["used_memory"].zh {
		t.Errorf("in Chinese: %q", got)
	}
}

// Before INFO comes, the page says it is reading; a server that refused it
// says why.
func TestRedisPanelBeforeInfo(t *testing.T) {
	_, tt, s := redisPanelApp(t, "")
	if !tt.HasText(locales.ReadingServerInfo()) {
		t.Errorf("no reading note; texts %q", tt.Texts())
	}
	s.redisInfoErr = errors.New("NOAUTH Authentication required")
	tt.Frame()
	if !strings.Contains(strings.Join(tt.Texts(), " "), "INFO") {
		t.Errorf("no error; texts %q", tt.Texts())
	}
}

// A section the server stops sending falls back to Server, and one chosen
// while it sends it is kept.
func TestRedisSectionShown(t *testing.T) {
	in := redis.ParseInfo(cacheInfo)
	a := &App{}
	if got := a.redisSectionShown(in); got != "Server" {
		t.Errorf("the first: %q", got)
	}
	a.redisSection = "Memory"
	if got := a.redisSectionShown(in); got != "Memory" {
		t.Errorf("the chosen: %q", got)
	}
	a.redisSection = "Commandstats"
	if got := a.redisSectionShown(in); got != "Server" {
		t.Errorf("a section the server did not send: %q", got)
	}
	if got := a.redisSectionShown(redis.Info{}); got != "" {
		t.Errorf("no sections: %q", got)
	}
}
