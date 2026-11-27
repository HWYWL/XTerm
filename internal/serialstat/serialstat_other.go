//go:build !windows

package serialstat

import gserial "go.bug.st/serial"

// lineStatus is unsupported where the system does not report the line's
// errors to the port: the pins and the bytes are still there, the counts
// are left to the panel to say it cannot show.
func lineStatus(gserial.Port) (f lineFlags, in, out int, ok bool) {
	return f, 0, 0, false
}
