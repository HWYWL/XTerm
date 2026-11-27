// Package serialstat reads what a serial port's driver reports of the line:
// the modem input pins, the errors the line has seen, and the bytes waiting
// in the driver's buffers. What a system does not report is left
// unsupported, so that the panel can say so rather than show a zero.
package serialstat

import (
	"errors"
	"sync"
	"time"

	gserial "go.bug.st/serial"
)

// ErrNoPort is a pin asked for with no port to raise it on.
var ErrNoPort = errors.New("serial status: no port")

// Bits are the modem input pins of the port: the ones a device or a modem
// raises, not the ones the app does.
type Bits struct {
	CTS, DSR, DCD, RI bool
}

// Status is what the driver said when it was last polled: the pins, the
// errors it has counted since the port opened, and the bytes waiting in its
// buffers. The OK fields say whether the system reports each part.
type Status struct {
	At        time.Time
	Bits      Bits
	BitsOK    bool
	Frame     uint64
	Parity    uint64
	Overrun   uint64
	Break     uint64
	ErrorsOK  bool
	InQueued  int
	OutQueued int
	QueuesOK  bool
}

// Probe polls a port's driver on the UI thread, once a second, and keeps
// what it has counted. A port the system does not report on is polled all
// the same: pins and counts stay unsupported.
type Probe struct {
	port gserial.Port

	mu      sync.Mutex
	st      Status
	frame   uint64
	parity  uint64
	overrun uint64
	brk     uint64
}

// New is a probe of port, which may be nil.
func New(port gserial.Port) *Probe { return &Probe{port: port} }

// Poll reads the pins and the driver's counters. The drivers clear their
// error flags as they are read, so the probe adds them up itself.
func (p *Probe) Poll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.port == nil {
		return
	}
	if bits, err := p.port.GetModemStatusBits(); err == nil && bits != nil {
		p.st.Bits = Bits{CTS: bits.CTS, DSR: bits.DSR, DCD: bits.DCD, RI: bits.RI}
		p.st.BitsOK, p.st.At = true, time.Now()
	}
	if f, in, out, ok := lineStatus(p.port); ok {
		if f.frame {
			p.frame++
		}
		if f.parity {
			p.parity++
		}
		if f.overrun {
			p.overrun++
		}
		if f.brk {
			p.brk++
		}
		p.st.Frame, p.st.Parity = p.frame, p.parity
		p.st.Overrun, p.st.Break = p.overrun, p.brk
		p.st.InQueued, p.st.OutQueued = in, out
		p.st.ErrorsOK, p.st.QueuesOK = true, true
	}
}

// Snapshot is what the last Poll found.
func (p *Probe) Snapshot() Status {
	if p == nil {
		return Status{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.st
}

// SetDTR raises or lowers the DTR pin.
func (p *Probe) SetDTR(on bool) error {
	if p == nil || p.port == nil {
		return ErrNoPort
	}
	return p.port.SetDTR(on)
}

// SetRTS raises or lowers the RTS pin.
func (p *Probe) SetRTS(on bool) error {
	if p == nil || p.port == nil {
		return ErrNoPort
	}
	return p.port.SetRTS(on)
}

// lineFlags are the line's errors the driver reports as pending.
type lineFlags struct{ frame, parity, overrun, brk bool }
