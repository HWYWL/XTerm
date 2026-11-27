package serialstat

import (
	"errors"
	"testing"

	gserial "go.bug.st/serial"
)

// fakePort is a port with the pins a test sets, and the DTR and RTS a test
// reads back.
type fakePort struct {
	gserial.Port // the rest is never asked for
	bits         gserial.ModemStatusBits
	dtr, rts     bool
	err          error
}

func (p *fakePort) GetModemStatusBits() (*gserial.ModemStatusBits, error) {
	if p.err != nil {
		return nil, p.err
	}
	b := p.bits
	return &b, nil
}

func (p *fakePort) SetDTR(on bool) error { p.dtr = on; return p.err }
func (p *fakePort) SetRTS(on bool) error { p.rts = on; return p.err }

// The pins are read, and a port that fails is left unsupported rather than
// counted.
func TestProbeReadsPins(t *testing.T) {
	port := &fakePort{bits: gserial.ModemStatusBits{CTS: true, DSR: true, DCD: true, RI: false}}
	p := New(port)
	p.Poll()
	st := p.Snapshot()
	if !st.BitsOK {
		t.Fatal("the pins are unsupported")
	}
	if !st.Bits.CTS || !st.Bits.DSR || !st.Bits.DCD || st.Bits.RI {
		t.Errorf("the pins %+v", st.Bits)
	}
	// A failing read leaves what was there.
	port.err = errors.New("no modem status")
	p.Poll()
	if st := p.Snapshot(); !st.BitsOK || !st.Bits.CTS {
		t.Errorf("the pins after a failure %+v", st)
	}
	// The counts are the driver's, which this port does not report.
	if st := p.Snapshot(); st.ErrorsOK || st.QueuesOK {
		t.Errorf("the counts of a port without a driver %+v", st)
	}
}

// DTR and RTS go to the port, and a probe with none says so.
func TestProbeSetsPins(t *testing.T) {
	port := &fakePort{}
	p := New(port)
	if err := p.SetDTR(false); err != nil {
		t.Fatal(err)
	}
	if err := p.SetRTS(false); err != nil {
		t.Fatal(err)
	}
	if port.dtr || port.rts {
		t.Errorf("the pins of the port %v %v", port.dtr, port.rts)
	}
	if err := New(nil).SetDTR(true); err == nil {
		t.Error("no port, and no error")
	}
}

// A probe with no port, or none at all, is polled and read without a crash.
func TestProbeWithoutPort(t *testing.T) {
	New(nil).Poll()
	var p *Probe
	p.Poll()
	if st := p.Snapshot(); st.BitsOK || st.ErrorsOK {
		t.Errorf("the status of no probe %+v", st)
	}
}
