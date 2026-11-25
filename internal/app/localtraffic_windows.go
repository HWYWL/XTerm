//go:build windows

package app

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ifTypeSoftwareLoopback is IF_TYPE_SOFTWARE_LOOPBACK: its traffic counts
// in both directions and is the app's own, not the network's.
const ifTypeSoftwareLoopback = 24

// localCounters sums what the interfaces that are up have received and
// sent so far, the loopback left out, as /proc/net/dev's lines do on
// Linux.
func localCounters() (rx, tx uint64, ok bool) {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(0, &table); err != nil || table == nil {
		return 0, 0, false
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	for _, row := range unsafe.Slice(&table.Table[0], int(table.NumEntries)) {
		if row.OperStatus != windows.IfOperStatusUp || row.Type == ifTypeSoftwareLoopback {
			continue
		}
		rx += row.InOctets
		tx += row.OutOctets
	}
	return rx, tx, true
}
