//go:build linux

package app

import "os"

// localCounters reads this computer's counters from /proc/net/dev, which
// every Linux system has: no library, no syscall of its own.
func localCounters() (rx, tx uint64, ok bool) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, false
	}
	rx, tx = parseNetDev(data)
	return rx, tx, true
}
