package app

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"finallyshell/internal/telnet"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The System page of a Telnet tab: the link and the options the server
// negotiated, what the connection carried, the errors the socket and the
// protocol saw, and the IAC commands of the stream. The terminal's
// goroutines count the bytes and the commands (internal/telnet); a ticker
// samples the rates, sends Telnet's NOP as a heartbeat and judges a link
// that has gone quiet, once a second.

// telnetMeterPoll is how often a Telnet tab's page samples; tests shorten
// it.
var telnetMeterPoll = time.Second

// telnetDialTimeout is how long a Telnet connection may take before it is
// reported as a connect timeout.
var telnetDialTimeout = 10 * time.Second

// The heartbeat, and the link that stopped answering it: telnetHeartbeatEvery
// is how often the NOP goes out, and telnetBeatWarn how many of them go
// unanswered before an idle socket is judged a half-open connection. Tests
// shorten both.
var (
	telnetHeartbeatEvery = 10 * time.Second
	telnetBeatWarn       = 3
	telnetIdleWarn       = 60 * time.Second
)

// telnetMeter is what a Telnet tab's System page shows (telnetpanel.go).
type telnetMeter struct {
	mu sync.Mutex
	// conn is the connection the page reports on: the pane's last opened.
	// A session keeps the meter across a reconnect, so the totals and the
	// counters below outlive it.
	conn *telnet.Conn
	addr string

	// The bytes of the connections that ended, and the last sample, from
	// which the rates are taken.
	rx, tx                   uint64
	sampledAt                time.Time
	lastTotalRx, lastTotalTx uint64
	rxHist, txHist           []float64
	peakRx, peakTx           float64

	// What the session has seen.
	attempts                  int
	dialTimeouts              int
	refused                   int
	readTimeouts              uint64
	halfOpens                 uint64
	halfOpen                  bool
	readErrSeen, writeErrSeen uint64
	lastReadErr               time.Time
	lastWriteErr              time.Time

	// The heartbeat: what the page sent, and what the server answered.
	beats       uint64
	beatsOK     uint64
	beatMisses  int
	beatPending bool
	beatAtRX    uint64
	lastBeat    time.Time
	lastBeatOK  time.Time

	connectedAt time.Time
	// lastRXSeen is the connection's RX at the sample before, which tells
	// a link that came back from a half-open one.
	lastRXSeen uint64
	st         telnet.Stats
	opts       telnet.Options
}

// telnetView is the meter as the page draws it: a copy, taken once a frame.
type telnetView struct {
	Addr           string
	RX, TX         uint64
	RXRate, TXRate float64
	RXHist, TXHist []float64
	PeakRX, PeakTX float64
	Opts           telnet.Options

	Beats      uint64
	BeatsOK    uint64
	BeatMisses int
	LastBeat   time.Time
	LastBeatOK time.Time

	Idle      time.Duration
	HalfOpen  bool
	HalfOpens uint64

	ReadTimeouts  uint64
	WriteTimeouts uint64
	Refused       int
	DialTimeouts  int
	Reconnects    int

	// LastReadErr and LastWriteErr are when the socket last failed in each
	// direction, which tells a link that died on a write from one that
	// only ran out of time reading.
	LastReadErr, LastWriteErr time.Time

	ST telnet.Stats
}

// telnetMeter is s's meter, made the first time it is asked for: a session
// keeps it across a reconnect, as it keeps its serial one.
func (a *App) telnetMeter(s *Session) *telnetMeter {
	if s.telnet == nil {
		s.telnet = &telnetMeter{}
	}
	return s.telnet
}

// noteAttempt counts a connection this session starts.
func (m *telnetMeter) noteAttempt() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.attempts++
	m.mu.Unlock()
}

// attach is the connection the page reports on, with its address: a
// session with several panes has one per pane, and the last opened is the
// one the page shows.
func (m *telnetMeter) attach(conn *telnet.Conn, addr string) {
	if m == nil || conn == nil {
		return
	}
	m.mu.Lock()
	if m.conn != nil {
		st := m.conn.Stats()
		m.rx += st.RX
		m.tx += st.TX
		// The page adds the session's bytes to the last snapshot's: what
		// was folded in must not be counted twice.
		m.st.RX, m.st.TX = 0, 0
	}
	m.conn, m.addr = conn, addr
	m.connectedAt = time.Now()
	m.beatMisses, m.beatPending, m.halfOpen = 0, false, false
	m.beatAtRX, m.lastRXSeen = 0, 0
	m.mu.Unlock()
}

// noteDial is a connection that failed: a timeout or a refused port.
func (m *telnetMeter) noteDial(err error) {
	if m == nil || err == nil {
		return
	}
	m.mu.Lock()
	switch {
	case telnetDialTimeoutErr(err):
		m.dialTimeouts++
	case telnetRefusedErr(err):
		m.refused++
	}
	m.mu.Unlock()
}

// detach forgets the connection, folding its bytes into the session's
// totals: the socket is closed, and the page must not read it any more.
func (m *telnetMeter) detach() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.conn != nil {
		st := m.conn.Stats()
		m.rx += st.RX
		m.tx += st.TX
		st.RX, st.TX = 0, 0 // folded in, and not counted twice
		m.st = st
		m.conn = nil
	}
	m.beatPending = false
	m.mu.Unlock()
}

// sample takes a second's reading: the rates, the curves, the heartbeat,
// and whether a quiet link has died.
func (m *telnetMeter) sample(now time.Time) {
	if m == nil {
		return
	}
	m.mu.Lock()
	conn := m.conn
	var st telnet.Stats
	if conn != nil {
		st = conn.Stats()
		m.st, m.opts = st, conn.Options()
	}
	totalRx, totalTx := m.rx+st.RX, m.tx+st.TX

	// The heartbeat: no answer to the one outstanding is a miss, and a
	// quiet socket with misses behind it is a half-open connection.
	if m.beatPending && now.Sub(m.lastBeat) > telnetBeatTimeout() {
		m.beatPending = false
		m.beatMisses++
	}
	answered := m.beatPending && st.RX > m.beatAtRX
	if answered {
		m.beatPending = false
		m.beatsOK++
		m.beatMisses = 0
		m.lastBeatOK = now
	}
	sendBeat := conn != nil && !m.beatPending && now.Sub(m.lastBeat) >= telnetHeartbeatEvery
	idle := time.Duration(0)
	if conn != nil {
		quietFrom := m.connectedAt
		if st.LastRX.After(quietFrom) {
			quietFrom = st.LastRX
		}
		idle = now.Sub(quietFrom)
		if st.ReadErrs+st.ReadTimeouts > m.readErrSeen {
			m.lastReadErr = now
		}
		if st.WriteErrs+st.WriteTimeouts > m.writeErrSeen {
			m.lastWriteErr = now
		}
		m.readErrSeen = st.ReadErrs + st.ReadTimeouts
		m.writeErrSeen = st.WriteErrs + st.WriteTimeouts
		dead := idle > telnetIdleWarn && m.beatMisses >= telnetBeatWarn
		if dead && !m.halfOpen {
			m.halfOpen = true
			m.halfOpens++
			m.readTimeouts++
		}
		if !dead && (answered || st.RX > m.lastRXSeen) {
			m.halfOpen = false
		}
		m.lastRXSeen = st.RX
	}
	if m.sampledAt.IsZero() {
		m.sampledAt, m.lastTotalRx, m.lastTotalTx = now, totalRx, totalTx
		m.mu.Unlock()
		m.sendBeat(conn, sendBeat, st.RX, now)
		return
	}
	seconds := now.Sub(m.sampledAt).Seconds()
	if seconds <= 0 {
		m.mu.Unlock()
		return
	}
	rxRate := float64(totalRx-m.lastTotalRx) / seconds
	txRate := float64(totalTx-m.lastTotalTx) / seconds
	m.rxHist = serialHist(m.rxHist, rxRate)
	m.txHist = serialHist(m.txHist, txRate)
	if rxRate > m.peakRx {
		m.peakRx = rxRate
	}
	if txRate > m.peakTx {
		m.peakTx = txRate
	}
	m.lastTotalRx, m.lastTotalTx, m.sampledAt = totalRx, totalTx, now
	m.mu.Unlock()
	m.sendBeat(conn, sendBeat, st.RX, now)
}

// sendBeat writes the heartbeat, if one is due, outside the lock: a write
// to a socket whose far side stopped reading must not hold the page.
func (m *telnetMeter) sendBeat(conn *telnet.Conn, due bool, rx uint64, now time.Time) {
	if !due || conn == nil {
		return
	}
	if err := conn.SendNOP(); err != nil {
		return
	}
	m.mu.Lock()
	m.beats++
	m.lastBeat = now
	m.beatPending = true
	m.beatAtRX = rx
	m.mu.Unlock()
}

// telnetBeatTimeout is how long an unanswered heartbeat waits before it
// counts as missed: a few heartbeats.
func telnetBeatTimeout() time.Duration {
	return time.Duration(telnetBeatWarn) * telnetHeartbeatEvery
}

// view is the meter's copy for the page.
func (m *telnetMeter) view() telnetView {
	if m == nil {
		return telnetView{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	v := telnetView{
		Addr: m.addr,
		RX:   m.rx + m.st.RX, TX: m.tx + m.st.TX,
		RXHist: m.rxHist, TXHist: m.txHist,
		PeakRX: m.peakRx, PeakTX: m.peakTx,
		Opts:       m.opts,
		Beats:      m.beats,
		BeatsOK:    m.beatsOK,
		BeatMisses: m.beatMisses,
		LastBeat:   m.lastBeat, LastBeatOK: m.lastBeatOK,
		HalfOpen: m.halfOpen, HalfOpens: m.halfOpens,
		ReadTimeouts:  m.readTimeouts + m.st.ReadTimeouts,
		WriteTimeouts: m.st.WriteTimeouts,
		Refused:       m.refused,
		DialTimeouts:  m.dialTimeouts,
		Reconnects:    max(0, m.attempts-1),
		LastReadErr:   m.lastReadErr,
		LastWriteErr:  m.lastWriteErr,
		ST:            m.st,
	}
	if n := len(m.rxHist); n > 0 {
		v.RXRate = m.rxHist[n-1]
	}
	if n := len(m.txHist); n > 0 {
		v.TXRate = m.txHist[n-1]
	}
	quietFrom := m.connectedAt
	if m.st.LastRX.After(quietFrom) {
		quietFrom = m.st.LastRX
	}
	if !quietFrom.IsZero() {
		v.Idle = time.Since(quietFrom)
	}
	return v
}

// startTelnetMeter samples s's meter once a second, until ctx ends, and
// redraws the page while the session is the one shown.
func (a *App) startTelnetMeter(s *Session, ctx context.Context) {
	m := a.telnetMeter(s)
	m.sample(time.Now())
	poll := telnetMeterPoll // read before the goroutine, which tests shorten
	go func() {
		t := time.NewTicker(poll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
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

// telnetDialTimeoutErr tells whether a failed connection ran out of time.
func telnetDialTimeoutErr(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout() || os.IsTimeout(err) ||
		errors.Is(err, context.DeadlineExceeded)
}

// telnetRefusedErr tells whether the server's port refused the connection.
func telnetRefusedErr(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) ||
		strings.Contains(strings.ToLower(err.Error()), "refused")
}

// The link as the page names it.
type telnetLink int

const (
	telnetLinkConnecting telnetLink = iota
	telnetLinkConnected
	telnetLinkDisconnected
	telnetLinkConnectTimeout
	telnetLinkReadTimeout
	telnetLinkWriteFailed
)

// telnetLinkOf is s's link now, from its session's state and its meter:
// a connection that went quiet past its heartbeats reads as a timeout,
// and one whose last failure was a write reads as a failed write.
func telnetLinkOf(s *Session, v telnetView) telnetLink {
	if s == nil {
		return telnetLinkDisconnected
	}
	recent := func(t time.Time) bool {
		return !t.IsZero() && time.Since(t) < time.Minute
	}
	switch s.status {
	case statusConnecting:
		return telnetLinkConnecting
	case statusConnected:
		switch {
		case v.HalfOpen:
			return telnetLinkReadTimeout
		case recent(v.LastWriteErr) && v.LastWriteErr.After(v.LastReadErr):
			return telnetLinkWriteFailed
		}
		return telnetLinkConnected
	case statusFailed:
		if telnetDialTimeoutErr(s.err) {
			return telnetLinkConnectTimeout
		}
	}
	switch {
	// A session the app ended itself is a plain disconnect: only one the
	// socket ended on its own is reported as a timeout or a failed write.
	case v.HalfOpen || s.err != nil && recent(v.LastReadErr) && v.LastReadErr.After(v.LastWriteErr):
		return telnetLinkReadTimeout
	case s.err != nil && recent(v.LastWriteErr):
		return telnetLinkWriteFailed
	}
	return telnetLinkDisconnected
}

// telnetOptionText is what the client answered an option the server asked
// for.
func telnetOptionText(o telnet.Option) string {
	switch o {
	case telnet.OptionOn:
		return locales.TelnetAgreed()
	case telnet.OptionRefused:
		return locales.TelnetOptRefused()
	}
	return "—"
}

// telnetOptionColor is green once agreed, red once refused.
func telnetOptionColor(o telnet.Option) ui.Color {
	switch o {
	case telnet.OptionOn:
		return assetGreen
	case telnet.OptionRefused:
		return assetOrange
	}
	return theme.TextMuted
}
