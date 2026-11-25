package app

import (
	"testing"

	"finallyshell/internal/monitor"
)

// TestNetToday: the day's traffic is what the interfaces read now less
// what they read at its first sample, kept for the next start, and a
// counter that went back starts the day again.
func TestNetToday(t *testing.T) {
	t.Setenv("FINALLYSHELL_DIR", t.TempDir())
	a := &App{netDay: loadNetDay()}
	today := a.netToday("h1")
	v := monitor.NetView{Names: []string{"eth0"}, RxTotal: 1_000, TxTotal: 100}
	if rx, tx := today(v); rx != 0 || tx != 0 {
		t.Fatalf("first sample: %d, %d", rx, tx)
	}

	v.RxTotal, v.TxTotal = 61_000, 2_200
	if rx, tx := today(v); rx != 60_000 || tx != 2_100 {
		t.Errorf("today: %d, %d", rx, tx)
	}

	// The bases are on disk: another run counts from the same start.
	b := &App{netDay: loadNetDay()}
	if rx, tx := b.netToday("h1")(v); rx != 60_000 || tx != 2_100 {
		t.Errorf("after a restart: %d, %d", rx, tx)
	}

	// A counter that went back, as an interface reset, starts it again.
	v.RxTotal = 10
	if rx, _ := b.netToday("h1")(v); rx != 0 {
		t.Errorf("after a reset: %d", rx)
	}

	// Another host, and another set of interfaces, keep their own.
	if rx, _ := a.netToday("h2")(v); rx != 0 {
		t.Errorf("another host: %d", rx)
	}
}
