package app

import (
	"strings"
	"testing"
	"time"

	"finallyshell/internal/monitor"
)

// The meter counts the device's bytes and frames, the lines they make, the
// line still unfinished, and the words of a crash, even split across two
// reads.
func TestSerialMeterReads(t *testing.T) {
	var m serialMeter
	m.scan.Store(true)
	first, second := []byte("panic: x\nhel"), []byte("lo\n")
	m.noteRead(first)
	m.noteRead(second)
	v := m.view()
	if v.RX != uint64(len(first)+len(second)) {
		t.Errorf("the bytes %d", v.RX)
	}
	if v.Frames != 2 {
		t.Errorf("the frames %d", v.Frames)
	}
	if v.Lines != 2 {
		t.Errorf("the lines %d", v.Lines)
	}
	if v.Tail != 0 {
		t.Errorf("the unfinished line %d", v.Tail)
	}
	if v.Keywords[0] != 1 {
		t.Errorf("the panics %v", v.Keywords)
	}
	// A word split across the reads still counts, once.
	m.noteRead([]byte("pa"))
	m.noteRead([]byte("nic\n"))
	if v := m.view(); v.Keywords[0] != 2 {
		t.Errorf("the split panic %v", v.Keywords)
	}
	// The switch stops the counting.
	m.scan.Store(false)
	m.noteRead([]byte("crash\n"))
	if v := m.view(); v.Keywords[1] != 0 {
		t.Errorf("the keyword of a switch off %v", v.Keywords)
	}
}

// The frames' timing and sizes are kept, and so is the tail of a line the
// device has not ended.
func TestSerialMeterFrames(t *testing.T) {
	var m serialMeter
	m.noteRead([]byte("a"))
	time.Sleep(2 * time.Millisecond)
	m.noteRead([]byte("bc"))
	v := m.view()
	if v.MaxFrame != 2 {
		t.Errorf("the largest frame %d", v.MaxFrame)
	}
	if v.AvgFrame != 1 {
		t.Errorf("the mean frame %d", v.AvgFrame)
	}
	if v.LastGap <= 0 || v.AvgGap <= 0 {
		t.Errorf("the gaps %v %v", v.LastGap, v.AvgGap)
	}
	if v.Tail != 3 {
		t.Errorf("the unfinished line %d", v.Tail)
	}
}

// The rates are the bytes between two samples over the seconds between
// them, and the day's totals add the same bytes.
func TestSerialMeterSamples(t *testing.T) {
	var m serialMeter
	t0 := time.Unix(1000, 0)
	m.sample(t0)
	m.noteRead(make([]byte, 100))
	m.noteWrite(40)
	m.sample(t0.Add(time.Second))
	v := m.view()
	if v.RX != 100 || v.TX != 40 {
		t.Errorf("the totals %d %d", v.RX, v.TX)
	}
	if v.RXRate != 100 || v.TXRate != 40 {
		t.Errorf("the rates %v %v", v.RXRate, v.TXRate)
	}
	if v.PeakRX != 100 || v.PeakTX != 40 {
		t.Errorf("the peaks %v %v", v.PeakRX, v.PeakTX)
	}
	if v.DayRX != 100 || v.DayTX != 40 {
		t.Errorf("today %d %d", v.DayRX, v.DayTX)
	}
	if len(v.RXHist) != 1 || len(v.TXHist) != 1 {
		t.Errorf("the curves %v %v", v.RXHist, v.TXHist)
	}
	// A second with nothing changes nothing.
	m.sample(t0.Add(2 * time.Second))
	quiet := m.view()
	if quiet.RXRate != 0 || quiet.TXRate != 0 {
		t.Errorf("the rates of a quiet second %v %v", quiet.RXRate, quiet.TXRate)
	}
	if len(quiet.RXHist) != 2 || len(quiet.TXHist) != 2 {
		t.Errorf("the curves %v %v", quiet.RXHist, quiet.TXHist)
	}
}

// The curves keep the window and no more.
func TestSerialHistWindow(t *testing.T) {
	var h []float64
	for i := 0; i < monitor.HistoryLen+10; i++ {
		h = serialHist(h, float64(i))
	}
	if len(h) != monitor.HistoryLen {
		t.Fatalf("the curve %d samples", len(h))
	}
	if h[len(h)-1] != float64(monitor.HistoryLen+9) {
		t.Errorf("the newest %v", h[len(h)-1])
	}
}

// A meter that was never sampled, or is not there, is read without a crash.
func TestSerialMeterEmpty(t *testing.T) {
	var m *serialMeter
	if v := m.view(); v.RX != 0 || v.Scan {
		t.Errorf("the view of no meter %+v", v)
	}
	m.sample(time.Now())
	var none serialMeter
	v := none.view()
	if v.RX != 0 || len(v.RXHist) != 0 {
		t.Errorf("the view of a new meter %+v", v)
	}
	if got := strings.TrimSpace(v.LastGap.String()); got != "0s" {
		t.Errorf("the gap of a new meter %q", got)
	}
}
