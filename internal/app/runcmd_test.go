package app

import (
	"strings"
	"testing"
	"unicode/utf8"

	"finallyshell/internal/config"
	"finallyshell/internal/sshtest"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The one-off command window runs a script on the session's host on its own
// connection, dials a host that has no session, and says so when it has no
// password to dial with.
func TestRunCmdWindow(t *testing.T) {
	srv := sshtest.Start(t)
	srv.ServeExec("from-the-server\n", "to-stderr\n", 0)
	a, p := testApp(t)
	t.Cleanup(a.CloseAll)
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()

	// Without a saved connection, the window says where to add one.
	a.openPlugin(pluginRunCmd)
	w := p.show(t, WindowRunCmd)
	w.Frame()
	if !w.HasText(locales.RunCmdNoHosts()) {
		t.Fatalf("no hint; texts %q", w.Texts())
	}

	// Two hosts on the same server: the first with a session, whose key the
	// connection then trusts, the second without one.
	host := func(id, name string) *config.Host {
		h := &config.Host{ID: id, Name: name, Addr: srv.Host, Port: srv.Port, User: "tester",
			Auth: config.AuthPassword, NoMonitor: true}
		a.hosts.Add(h)
		a.passwords[id] = sshtest.Password
		return h
	}
	first := host("one", "web-1")
	host("two", "web-2")
	a.openSession(first)
	s := a.current()
	if answerHostKeys(t, p, s); s.status != statusConnected {
		t.Fatalf("connecting: %s", describeError(s.err))
	}

	run := func(id, script string) {
		t.Helper()
		a.runCmd.host, a.runCmd.script = id, script
		w.Frame()
		if err := w.Click(locales.RunCmdRun()); err != nil {
			t.Fatal(err)
		}
		p.until(t, "the script's output", func() bool { return !a.runCmd.busy })
	}

	// The connected host runs it on the session's connection, stderr included,
	// and no new login is made for it.
	logins := srv.Logins()
	run("one", "echo one\n")
	if !strings.Contains(a.runCmd.out, "from-the-server") || !strings.Contains(a.runCmd.out, "to-stderr") {
		t.Errorf("output %q", a.runCmd.out)
	}
	if a.runCmd.err != "" || a.runCmd.who != "web-1" {
		t.Errorf("err %q, who %q", a.runCmd.err, a.runCmd.who)
	}
	if srv.Logins() != logins {
		t.Error("the session's host logged in again")
	}

	// The host without a session is dialed, with the key the first connection
	// trusted, and the connection closes after.
	run("two", "echo two\n")
	if !strings.Contains(a.runCmd.out, "from-the-server") || a.runCmd.err != "" {
		t.Errorf("output %q, err %q", a.runCmd.out, a.runCmd.err)
	}
	if srv.Logins() != logins+1 {
		t.Errorf("%d logins for a host without a session", srv.Logins()-logins)
	}

	// A host the app has no password for says so instead of asking.
	a.hosts.Add(&config.Host{ID: "three", Name: "db-1", Addr: srv.Host, Port: srv.Port,
		User: "tester", Auth: config.AuthPassword, NoMonitor: true})
	before := a.sessions
	run("three", "echo never\n")
	if a.runCmd.err != errNoSecret.Error() {
		t.Errorf("err %q", a.runCmd.err)
	}
	if len(a.sessions) != len(before) {
		t.Error("a run on a host opened a session")
	}
}

// A long output keeps its end, from the start of a line, and a cut inside a
// character drops its tail rather than showing it broken.
func TestClipRunCmd(t *testing.T) {
	short := "one\ntwo\n"
	if got, clipped := clipRunCmd(short); got != short || clipped {
		t.Errorf("short: %q %v", got, clipped)
	}

	long := strings.Repeat("x", runCmdOutLimit) + "\nlast\n"
	if got, clipped := clipRunCmd(long); !clipped || got != "last\n" {
		t.Errorf("long: %q %v", got, clipped)
	}

	// The first byte left of a 3-byte character: the other two go with the
	// bytes left out.
	n := runCmdOutLimit - 2
	broken := strings.Repeat("a", n) + "中" + strings.Repeat("b", n)
	got, clipped := clipRunCmd(broken)
	if !clipped || !utf8.ValidString(got) || !strings.HasPrefix(got, "b") {
		t.Errorf("a cut in a character: %q", got)
	}
}
