package app

import (
	"strconv"
	"strings"
	"time"

	"finallyshell/internal/monitor"
)

// This computer's traffic, as the monitor shows it beside the server's:
// QuickTerm's panel has both, the server's graph and the one of the
// machine the app runs on. Nothing of it comes over the connection: the
// counters are the system's own, read here.

// localInterval is how often the counters are read: the server's stats
// come every 2 seconds, and the two graphs are read together.
const localInterval = 2 * time.Second

// localTraffic is this computer's traffic: the counters of the last
// sample, the speeds since it, and the history the graph draws, newest
// last. It lives on the UI thread alone — the sampler reads the counters
// on its own goroutine and posts them.
type localTraffic struct {
	ok       bool // a sample is in: the speeds and the history mean something
	rx, tx   uint64
	at       time.Time
	up, down float64 // bytes per second
	upHist   []float64
	downHist []float64
}

// WatchLocalTraffic samples this computer's traffic every localInterval
// until the app quits. The main window calls it as it starts: the samples
// are cheap, and the monitor may be asked for them at any frame.
func (a *App) WatchLocalTraffic() {
	if a.localWatching {
		return
	}
	a.localWatching = true
	go func() {
		t := time.NewTicker(localInterval)
		defer t.Stop()
		for range t.C {
			rx, tx, ok := localCounters()
			at := time.Now()
			a.post(func() { a.addLocalTraffic(rx, tx, at, ok) })
		}
	}()
}

// addLocalTraffic takes one sample of the counters, on the UI thread, and
// works out the speeds since the last: a counter that went back (an
// interface reset) leaves the speed at its last value.
func (a *App) addLocalTraffic(rx, tx uint64, at time.Time, ok bool) {
	lt := &a.localNet
	if !ok {
		lt.ok = false
		return
	}
	if lt.ok && at.After(lt.at) {
		dt := at.Sub(lt.at).Seconds()
		if rx >= lt.rx {
			lt.down = float64(rx-lt.rx) / dt
		}
		if tx >= lt.tx {
			lt.up = float64(tx-lt.tx) / dt
		}
		lt.downHist = pushLocalHist(lt.downHist, lt.down)
		lt.upHist = pushLocalHist(lt.upHist, lt.up)
	}
	lt.rx, lt.tx, lt.at, lt.ok = rx, tx, at, true
}

// pushLocalHist appends v, keeping the last monitor.HistoryLen samples.
func pushLocalHist(hist []float64, v float64) []float64 {
	hist = append(hist, v)
	if len(hist) > monitor.HistoryLen {
		hist = hist[len(hist)-monitor.HistoryLen:]
	}
	return hist
}

// parseNetDev sums the bytes received and sent of the interfaces in
// /proc/net/dev, the loopback left out: its traffic is the app's own
// connections, counted in both directions, and no network's.
func parseNetDev(data []byte) (rx, tx uint64) {
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(name) == "lo" {
			continue
		}
		f := strings.Fields(rest)
		// receive: bytes packets errs drop fifo frame compressed multicast
		// transmit: bytes packets errs drop fifo colls carrier compressed
		if len(f) < 9 {
			continue
		}
		r, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		t, err := strconv.ParseUint(f[8], 10, 64)
		if err != nil {
			continue
		}
		rx += r
		tx += t
	}
	return rx, tx
}
