package app

import (
	"testing"
	"time"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// TestSafeHost: only what may go into a shell command as a host passes.
func TestSafeHost(t *testing.T) {
	for _, good := range []string{"10.0.0.1", "example.com", "fe80::1%eth0", "my_host-1"} {
		if !safeHost(good) {
			t.Errorf("%q was refused", good)
		}
	}
	for _, bad := range []string{"", "a b", "a;rm -rf /", "a|b", "a$(id)"} {
		if safeHost(bad) {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// TestDiagLatencyText: what the round trip reads, measured or not.
func TestDiagLatencyText(t *testing.T) {
	if got := diagLatencyText(0, false); got != locales.DiagLatencyUnmeasured() {
		t.Errorf("unmeasured %q", got)
	}
	if got := diagLatencyText(0, true); got != locales.DiagLatencyMeasuring() {
		t.Errorf("measuring %q", got)
	}
	if got := diagLatencyText(4200*time.Microsecond, false); got != "4.2 ms" {
		t.Errorf("measured %q", got)
	}
}

// TestDiagDialog: the window shows the session, the checks and their
// buttons.
func TestDiagDialog(t *testing.T) {
	a, _, s, _, tt := processesApp(t)
	s.Host = &config.Host{Name: "web-1", Addr: "10.0.0.1", Port: 22, User: "root"}
	s.status = statusConnected
	a.openDiag(s)
	tt.Frame()
	for _, want := range []string{
		locales.DiagTitle(), locales.DiagLatency(), locales.DiagPorts(),
		locales.DiagDNS(), locales.DiagHops(), locales.DiagAll(),
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, tt.Texts())
		}
	}
	saveShot(t, tt, "conn-diag-en.png")
	a.diag = nil
}
