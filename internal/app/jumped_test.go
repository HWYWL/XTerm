package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// A shell that ran ssh — a jump host's menu does — says so in the monitor:
// what the app shows is the host it connected to, and a connection through
// it reaches the other.
func TestJumpedSessionHint(t *testing.T) {
	a, tt, s := monitorPartsApp(t, "en")
	if s.jumpedFrom() {
		t.Fatal("the demo session jumped")
	}
	if tt.HasText(locales.JumpedSession()) {
		t.Fatalf("the hint before a jump; texts %q", tt.Texts())
	}
	s.panes.panes()[0].shell.comm = "ssh"
	tt.Frame()
	if !tt.HasText(locales.JumpedSession()) {
		t.Fatalf("no hint; texts %q", tt.Texts())
	}
	saveShot(t, tt, "monitor-jumped-en.png")
	if err := tt.Click(locales.NewConnectionThrough()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	if !a.windowOpen(WindowEditor) {
		t.Fatal("no editor")
	}
	if a.editor.proxy.kind != config.ProxyJump || a.editor.proxy.jump != s.Host.ID {
		t.Errorf("proxy %+v, want a jump through %q", a.editor.proxy, s.Host.ID)
	}
}

// Only a shell of an SSH host can have gone somewhere else: a local
// terminal runs no servers, and a Redis tab has no shell.
func TestJumpedFrom(t *testing.T) {
	s := &Session{Host: &config.Host{ID: "h1", Name: "bastion", Addr: "203.0.113.5", User: "jump"}}
	p := newPane()
	s.panes.reset(p)
	for _, comm := range []string{"bash", "vim", "ssh-agent", ""} {
		p.shell.comm = comm
		if s.jumpedFrom() {
			t.Errorf("%q counts as a jump", comm)
		}
	}
	for _, comm := range []string{"ssh", "sshpass", "mosh-client", "telnet"} {
		p.shell.comm = comm
		if !s.jumpedFrom() {
			t.Errorf("%q does not count as a jump", comm)
		}
	}
	s.Host.Protocol = config.ProtocolRedis
	if s.jumpedFrom() {
		t.Error("a Redis session jumped")
	}
	local := &Session{local: true}
	localHost := newPane()
	local.panes.reset(localHost)
	localHost.shell.comm = "ssh"
	if local.jumpedFrom() {
		t.Error("a local session jumped")
	}
}
