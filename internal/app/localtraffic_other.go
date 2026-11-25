//go:build !windows && !linux

package app

// localCounters has no way to read this system's counters, so the monitor
// leaves this computer's traffic out: the graph stays empty.
func localCounters() (rx, tx uint64, ok bool) { return 0, 0, false }
