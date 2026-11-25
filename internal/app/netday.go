package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/monitor"
)

// The traffic of a server since the day began, as the network card shows
// it under the graph: "今日 RX 18G · TX 2.1G". The server's counters run
// since it started, so the bytes of the day are what they read now less
// what they read at the day's first sample here. The bases are kept in
// the config directory, one per host and set of interfaces: a new day,
// or a counter that went back, starts them again.

// netDay is on disk: the day the bases are of, and per key the counters
// at its first sample, as received then sent.
type netDay struct {
	Date string               `json:"date"`
	Base map[string][2]uint64 `json:"base,omitempty"`
}

// netDayPath is where the bases are kept, beside the settings.
func netDayPath() string { return filepath.Join(config.Dir(), "netday.json") }

// loadNetDay reads the bases, none for a new day or a file that is not
// there: the next sample starts them.
func loadNetDay() netDay {
	d := netDay{Date: time.Now().Format(time.DateOnly), Base: map[string][2]uint64{}}
	data, err := os.ReadFile(netDayPath())
	if err != nil {
		return d
	}
	var saved netDay
	if json.Unmarshal(data, &saved) != nil || saved.Date != d.Date {
		return d
	}
	if saved.Base != nil {
		d.Base = saved.Base
	}
	return d
}

// saveNetDay writes the bases.
func saveNetDay(d netDay) {
	_ = os.MkdirAll(config.Dir(), 0o700)
	if data, err := json.MarshalIndent(d, "", "  "); err == nil {
		_ = os.WriteFile(netDayPath(), data, 0o600)
	}
}

// netToday is how many bytes the interfaces a host's graph shows have
// received and sent today; the view's counters start the day's base the
// first time it shows them.
func (a *App) netToday(hostID string) func(v monitor.NetView) (uint64, uint64) {
	return func(v monitor.NetView) (rx, tx uint64) {
		if len(v.Names) == 0 {
			return 0, 0
		}
		key := hostID + "|" + strings.Join(v.Names, "+")
		base, ok := a.netDay.Base[key]
		changed := false
		switch {
		case !ok, v.RxTotal < base[0], v.TxTotal < base[1]:
			base = [2]uint64{v.RxTotal, v.TxTotal}
			a.netDay.Base[key] = base
			changed = true
		}
		if changed {
			saveNetDay(a.netDay)
		}
		return v.RxTotal - base[0], v.TxTotal - base[1]
	}
}
