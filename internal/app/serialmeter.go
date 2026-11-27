package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/monitor"
	"finallyshell/internal/serial"
	"finallyshell/internal/serialstat"
)

// errNoPort is a pin asked for with no port open to raise it on.
var errNoPort = errors.New("no serial port is open")

// The System page of a serial tab: what the port carried, how it came in
// time, the errors the line and the app have seen, and what the driver says
// of its pins and its buffers. The terminal's goroutines note each read and
// write; a ticker samples the rates, the pins and the driver's counters on
// the UI thread, once a second.

// serialMeterPoll is how often a serial tab's page samples; tests shorten
// it.
var serialMeterPoll = time.Second

const (
	// serialIdleWarn is how long a device may be quiet before the page says
	// it has stopped: far past any console's pause.
	serialIdleWarn = 5 * time.Second
	// serialInQueueWarn is the driver's read queue past which the app is
	// said to be behind the device — the usual 4 KB buffer, filled.
	serialInQueueWarn = 4096
	// serialPendingWarn is the queued write past which the page warns: the
	// device is not taking bytes.
	serialPendingWarn = 4096
	// serialSaveEvery is how often the day's totals are written.
	serialSaveEvery = 10 * time.Second
	// serialKeywordCarry is how many bytes of a read are kept so that a
	// keyword that straddles two reads is still counted.
	serialKeywordCarry = 4
)

// serialMeter is what a serial tab's System page shows (serialpanel.go).
type serialMeter struct {
	app  *App
	host string

	mu sync.Mutex
	// What the port carried.
	rx, tx    uint64
	frames    uint64
	lines     uint64
	tail      int // the bytes of the line the device is still writing
	keywords  [3]uint64
	carry     []byte
	readErrs  uint64
	drops     uint64
	lastFrame time.Time
	lastGap   time.Duration
	maxGap    time.Duration
	sumGap    time.Duration
	gaps      int
	maxFrame  int
	sumFrame  int
	// The sampling: the rates of the last second, the curves, and today.
	sampledAt      time.Time
	lastRx, lastTx uint64
	rxHist, txHist []float64
	peakRx, peakTx float64
	day            string
	dayRx, dayTx   uint64
	// What the driver said at the last poll.
	probe                       *serialstat.Probe
	bits                        serialstat.Bits
	bitsOK                      bool
	frameErr, parityErr         uint64
	overrunErr, breakErr        uint64
	errsOK                      bool
	inQueue, outQueue           int
	queuesOK                    bool
	pendingChunks, pendingBytes int
	// DTR and RTS: the app raises and lowers them, so it knows them; the
	// library offers no way to read them back.
	dtr, rts bool
	wires    []*asyncConn
	// scan is whether the keywords are counted, which the page switches.
	scan atomic.Bool
}

// serialView is the meter as the page draws it: a copy, taken once a frame.
type serialView struct {
	RX, TX         uint64
	RXRate, TXRate float64
	RXHist, TXHist []float64
	PeakRX, PeakTX float64
	DayRX, DayTX   uint64
	Frames         uint64
	Lines          uint64
	Tail           int
	Keywords       [3]uint64
	Scan           bool
	ReadErrs       uint64
	Drops          uint64
	LastGap        time.Duration
	AvgGap         time.Duration
	MaxGap         time.Duration
	LastFrame      time.Time
	MaxFrame       int
	AvgFrame       int
	FrameErr       uint64
	ParityErr      uint64
	OverrunErr     uint64
	BreakErr       uint64
	ErrsOK         bool
	InQueue        int
	OutQueue       int
	QueuesOK       bool
	PendingChunks  int
	PendingBytes   int
	Bits           serialstat.Bits
	BitsOK         bool
	DTR, RTS       bool
	At             time.Time
}

// serialMeter is s's meter, made the first time it is asked for, with
// today's bytes of its host from the file: a session keeps it across a
// reconnect.
func (a *App) serialMeter(s *Session) *serialMeter {
	if s.serial == nil {
		host := ""
		if s.Host != nil {
			host = s.Host.ID
		}
		m := &serialMeter{app: a, host: host, dtr: true, rts: true}
		m.scan.Store(true)
		m.day = time.Now().Format(time.DateOnly)
		m.dayRx, m.dayTx = a.serialDayBytes(host)
		s.serial = m
	}
	return s.serial
}

// noteRead is a read of the device's: its bytes, its frame, the gap since
// the frame before, the line it leaves unfinished, and its keywords.
func (m *serialMeter) noteRead(data []byte) {
	if m == nil || len(data) == 0 {
		return
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rx += uint64(len(data))
	m.frames++
	m.sumFrame += len(data)
	if len(data) > m.maxFrame {
		m.maxFrame = len(data)
	}
	if !m.lastFrame.IsZero() {
		gap := now.Sub(m.lastFrame)
		m.lastGap = gap
		m.sumGap += gap
		m.gaps++
		if gap > m.maxGap {
			m.maxGap = gap
		}
	}
	m.lastFrame = now
	m.lines += uint64(bytes.Count(data, []byte{'\n'}))
	if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
		m.tail = len(data) - i - 1
	} else {
		m.tail += len(data)
	}
	if m.scan.Load() {
		m.countKeywords(data)
	}
}

// countKeywords counts the words of a crash in what the device sent,
// keeping a few bytes of the read before so that one split across two reads
// still counts.
func (m *serialMeter) countKeywords(data []byte) {
	if len(m.carry)+len(data) == 0 {
		return
	}
	buf := make([]byte, 0, len(m.carry)+len(data))
	buf = append(buf, m.carry...)
	for _, b := range data {
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		buf = append(buf, b)
	}
	for i, kw := range []string{"panic", "crash", "error"} {
		m.keywords[i] += uint64(bytes.Count(buf, []byte(kw)))
	}
	keep := min(len(buf), serialKeywordCarry)
	m.carry = append(m.carry[:0], buf[len(buf)-keep:]...)
}

// noteWrite is bytes the port took.
func (m *serialMeter) noteWrite(n int) {
	if m == nil || n <= 0 {
		return
	}
	m.mu.Lock()
	m.tx += uint64(n)
	m.mu.Unlock()
}

// noteDrop is a write the queue could not keep: the device has stopped
// taking bytes.
func (m *serialMeter) noteDrop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.drops++
	m.mu.Unlock()
}

// noteReadErr is a read the port failed, which the terminal takes as the
// connection's end.
func (m *serialMeter) noteReadErr() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.readErrs++
	m.mu.Unlock()
}

// attach is the port the driver is asked about: the pins, the errors and
// the buffers. A pane of its own has a port of its own, so the last one
// opened is the one the page reports on.
func (m *serialMeter) attach(p serial.Port) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.probe = serialstat.New(p)
	// A port opens with DTR and RTS high, until the page lowers them.
	m.dtr, m.rts = true, true
	m.mu.Unlock()
}

// addWire is one of the session's write queues, whose backlog the page
// shows.
func (m *serialMeter) addWire(w *asyncConn) {
	if m == nil || w == nil {
		return
	}
	m.mu.Lock()
	m.wires = append(m.wires, w)
	m.mu.Unlock()
}

// detach forgets the port the page was reading: it is closed, and the
// driver's pins, errors and buffers must not outlive it.
func (m *serialMeter) detach() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.probe = nil
	m.bitsOK, m.errsOK, m.queuesOK = false, false, false
	m.wires = nil
	m.pendingChunks, m.pendingBytes = 0, 0
	m.mu.Unlock()
}

// setDTR raises or lowers the DTR pin, and keeps what it was set to.
func (m *serialMeter) setDTR(on bool) error {
	if m == nil {
		return errNoPort
	}
	m.mu.Lock()
	p := m.probe
	m.mu.Unlock()
	if p == nil {
		return errNoPort
	}
	if err := p.SetDTR(on); err != nil {
		return err
	}
	m.mu.Lock()
	m.dtr = on
	m.mu.Unlock()
	return nil
}

// setRTS raises or lowers the RTS pin.
func (m *serialMeter) setRTS(on bool) error {
	if m == nil {
		return errNoPort
	}
	m.mu.Lock()
	p := m.probe
	m.mu.Unlock()
	if p == nil {
		return errNoPort
	}
	if err := p.SetRTS(on); err != nil {
		return err
	}
	m.mu.Lock()
	m.rts = on
	m.mu.Unlock()
	return nil
}

// sample takes a second's reading: the rates from the counters, the curves,
// today's totals, and the driver's pins, errors and buffers.
func (m *serialMeter) sample(now time.Time) {
	if m == nil {
		return
	}
	m.mu.Lock()
	probe := m.probe
	if probe != nil {
		probe.Poll()
		st := probe.Snapshot()
		if st.BitsOK {
			m.bits, m.bitsOK = st.Bits, true
		}
		if st.ErrorsOK {
			m.frameErr, m.parityErr = st.Frame, st.Parity
			m.overrunErr, m.breakErr = st.Overrun, st.Break
			m.errsOK = true
		}
		if st.QueuesOK {
			m.inQueue, m.outQueue = st.InQueued, st.OutQueued
			m.queuesOK = true
		}
	}
	m.pendingChunks, m.pendingBytes = 0, 0
	for _, w := range m.wires {
		chunks, bytes := w.Pending()
		m.pendingChunks += chunks
		m.pendingBytes += bytes
	}
	rx, tx := m.rx, m.tx
	if m.sampledAt.IsZero() {
		m.sampledAt, m.lastRx, m.lastTx = now, rx, tx
		m.mu.Unlock()
		return
	}
	seconds := now.Sub(m.sampledAt).Seconds()
	if seconds <= 0 {
		m.mu.Unlock()
		return
	}
	rxRate := float64(rx-m.lastRx) / seconds
	txRate := float64(tx-m.lastTx) / seconds
	m.rxHist = serialHist(m.rxHist, rxRate)
	m.txHist = serialHist(m.txHist, txRate)
	if rxRate > m.peakRx {
		m.peakRx = rxRate
	}
	if txRate > m.peakTx {
		m.peakTx = txRate
	}
	rxThen, txThen := rx-m.lastRx, tx-m.lastTx
	m.lastRx, m.lastTx, m.sampledAt = rx, tx, now
	day := now.Format(time.DateOnly)
	if day != m.day {
		m.day, m.dayRx, m.dayTx = day, 0, 0
	}
	m.dayRx += rxThen
	m.dayTx += txThen
	m.mu.Unlock()
	if m.app != nil {
		m.app.addSerialDay(m.host, rxThen, txThen)
	}
}

// view is the meter's copy for the page.
func (m *serialMeter) view() serialView {
	if m == nil {
		return serialView{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	v := serialView{
		RX: m.rx, TX: m.tx,
		RXHist: m.rxHist, TXHist: m.txHist,
		PeakRX: m.peakRx, PeakTX: m.peakTx,
		DayRX: m.dayRx, DayTX: m.dayTx,
		Frames: m.frames, Lines: m.lines, Tail: m.tail,
		Keywords: m.keywords, Scan: m.scan.Load(),
		ReadErrs: m.readErrs, Drops: m.drops,
		LastGap: m.lastGap, MaxGap: m.maxGap, LastFrame: m.lastFrame,
		MaxFrame: m.maxFrame,
		FrameErr: m.frameErr, ParityErr: m.parityErr,
		OverrunErr: m.overrunErr, BreakErr: m.breakErr, ErrsOK: m.errsOK,
		InQueue: m.inQueue, OutQueue: m.outQueue, QueuesOK: m.queuesOK,
		PendingChunks: m.pendingChunks, PendingBytes: m.pendingBytes,
		Bits: m.bits, BitsOK: m.bitsOK,
		DTR: m.dtr, RTS: m.rts,
	}
	if n := len(m.rxHist); n > 0 {
		v.RXRate = m.rxHist[n-1]
	}
	if n := len(m.txHist); n > 0 {
		v.TXRate = m.txHist[n-1]
	}
	if m.gaps > 0 {
		v.AvgGap = m.sumGap / time.Duration(m.gaps)
	}
	if m.frames > 0 {
		v.AvgFrame = m.sumFrame / int(m.frames)
	}
	return v
}

// startSerialMeter samples s's meter once a second, until ctx ends, and
// redraws the page while the session is the one shown.
func (a *App) startSerialMeter(s *Session, ctx context.Context) {
	m := a.serialMeter(s)
	m.sample(time.Now())
	poll := serialMeterPoll // read before the goroutine, which tests shorten
	go func() {
		t := time.NewTicker(poll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				a.post(func() { a.saveSerialDay() })
				return
			case <-t.C:
				a.post(func() {
					if ctx.Err() != nil {
						return
					}
					m.sample(time.Now())
					if a.connSession() == s {
						a.redraw()
					}
				})
			}
		}
	}()
}

// serialHist is the curves: the samples of the last window, newest last.
func serialHist(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > monitor.HistoryLen {
		h = h[len(h)-monitor.HistoryLen:]
	}
	return h
}

// serialDay is today's bytes of the serial ports, by host, on disk: a new
// day, or a host's counters that went back, starts them again.
type serialDay struct {
	Date  string               `json:"date"`
	Hosts map[string][2]uint64 `json:"hosts,omitempty"`
}

// serialDayPath is where the day's bytes are kept, beside the settings.
func serialDayPath() string { return filepath.Join(config.Dir(), "serialday.json") }

// serialDayBytes is today's bytes of a host's port, as they were left.
func (a *App) serialDayBytes(host string) (rx, tx uint64) {
	a.readSerialDay()
	if v, ok := a.serialDay.Hosts[host]; ok {
		return v[0], v[1]
	}
	return 0, 0
}

// addSerialDay adds a second's bytes to today's totals, and writes them
// from time to time.
func (a *App) addSerialDay(host string, rx, tx uint64) {
	if rx == 0 && tx == 0 {
		return
	}
	a.readSerialDay()
	v := a.serialDay.Hosts[host]
	v[0] += rx
	v[1] += tx
	a.serialDay.Hosts[host] = v
	if time.Since(a.serialDaySaved) >= serialSaveEvery {
		a.saveSerialDay()
	}
}

// readSerialDay reads the file once, and on a new day empties it.
func (a *App) readSerialDay() {
	today := time.Now().Format(time.DateOnly)
	if a.serialDayRead {
		if a.serialDay.Date != today {
			a.serialDay = serialDay{Date: today, Hosts: map[string][2]uint64{}}
			a.saveSerialDay()
		}
		return
	}
	a.serialDayRead = true
	a.serialDay = serialDay{Date: today, Hosts: map[string][2]uint64{}}
	if data, err := os.ReadFile(serialDayPath()); err == nil {
		var saved serialDay
		if json.Unmarshal(data, &saved) == nil && saved.Date == today && saved.Hosts != nil {
			a.serialDay.Hosts = saved.Hosts
		}
	}
}

// saveSerialDay writes today's bytes.
func (a *App) saveSerialDay() {
	a.serialDaySaved = time.Now()
	data, err := json.MarshalIndent(a.serialDay, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(config.Dir(), 0o700)
	_ = os.WriteFile(serialDayPath(), data, 0o600)
}
