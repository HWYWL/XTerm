//go:build windows

package serialstat

import (
	"reflect"

	"golang.org/x/sys/windows"

	gserial "go.bug.st/serial"
)

// The flags ClearCommError reports for a line, and clears as it reports
// them: the parity, framing and overrun errors of the driver, and a break.
const (
	ceRXOver   = 0x0001 // the driver's buffer was full: the app read too slowly
	ceOverrun  = 0x0002 // the hardware's shift register overran
	ceRXParity = 0x0004
	ceFrame    = 0x0008
	ceBreak    = 0x0010
)

// lineStatus reads the driver's pending error flags and its buffer sizes.
func lineStatus(port gserial.Port) (f lineFlags, in, out int, ok bool) {
	h, ok := handleOf(port)
	if !ok {
		return f, 0, 0, false
	}
	var errs uint32
	var st windows.ComStat
	if err := windows.ClearCommError(h, &errs, &st); err != nil {
		return f, 0, 0, false
	}
	f = lineFlags{
		frame:   errs&ceFrame != 0,
		parity:  errs&ceRXParity != 0,
		overrun: errs&(ceOverrun|ceRXOver) != 0,
		brk:     errs&ceBreak != 0,
	}
	return f, int(st.CBInQue), int(st.CBOutQue), true
}

// handleOf is the system handle the library's port holds: the library keeps
// it to itself, so it is read from the port's own fields. A version of the
// library that renames the field falls back to no counters, rather than to
// a crash.
func handleOf(port any) (windows.Handle, bool) {
	v := reflect.ValueOf(port)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return 0, false
	}
	f := v.Elem().FieldByName("handle")
	if !f.IsValid() || !f.CanUint() {
		return 0, false
	}
	return windows.Handle(f.Uint()), true
}
