// Package telnet is the client of a Telnet server: its terminal is the
// connection itself, once the negotiations the protocol puts in the stream
// are answered and taken out of it. It counts what the connection carried
// and the IAC commands it saw, for the System page of its tab, and sends
// the protocol's NOP as a heartbeat.
package telnet

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// The commands of Telnet, as RFC 854 numbers them, and the options the
// client answers.
const (
	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	ga   = 249
	se   = 240
	nop  = 241

	optBinary   = 0
	optEcho     = 1
	optSGA      = 3
	optTTYPE    = 24
	optNAWS     = 31
	optLinemode = 34

	ttypeIs   = 0
	ttypeSend = 1
)

// Option is what the client answered a server that asked for an option.
type Option uint8

const (
	// OptionNone is an option neither side asked for.
	OptionNone Option = iota
	// OptionOn is an option the client agreed to.
	OptionOn
	// OptionRefused is an option the client answered WONT or DONT.
	OptionRefused
)

// Options are the negotiations the System page shows: whether the server
// echoes, whether both sides suppress the go-ahead, and whether the server
// asked for the line mode, which the client refuses.
type Options struct {
	Echo, SGA, Linemode Option
}

// Stats is what a connection carried and what the protocol put in it, as
// the System page of its tab shows it.
type Stats struct {
	// RX and TX are the bytes of the terminal's, the negotiations taken
	// out; WireRX and WireTX count the bytes on the socket, IAC sequences
	// included.
	RX, TX         uint64
	WireRX, WireTX uint64
	// The IAC commands the server sent: the heartbeat's NOP, the
	// go-ahead, and the four negotiations, with the subnegotiations.
	NOP, GA, Subneg      uint64
	WILL, WONT, DO, DONT uint64
	// The IAC commands the client sent.
	SentWILL, SentWONT, SentDO, SentDONT uint64
	// Echo and SGA are the negotiations that mentioned those options;
	// NOPsSent is the heartbeats the page has sent.
	Echo, SGA, NOPsSent uint64
	// InvalidIAC is an IAC followed by no command the protocol has, and
	// Malformed a subnegotiation that never named its option or held a
	// command inside it.
	InvalidIAC, Malformed uint64
	// The socket's errors.
	ReadErrs, WriteErrs         uint64
	ReadTimeouts, WriteTimeouts uint64
	Resets                      uint64
	// LastRX and LastTX are when the socket last carried a byte, and
	// LastNOP when the client last sent its heartbeat.
	LastRX, LastTX, LastNOP time.Time
	LastErr                 string
	Closed                  bool
}

// Conn is a Telnet connection: it reads what the server sends without the
// negotiations, and writes what the terminal types, escaped as the
// protocol wants it.
type Conn struct {
	net.Conn

	r    *bufio.Reader
	mu   sync.Mutex // the writes, and the negotiations with them
	term string

	// The state of the reader: where in a command it is.
	state  byte
	verb   byte
	subOpt int
	sub    []byte

	naws  bool
	cols  int
	rows  int
	wrote int // the number of the option bytes the last resize wrote

	// What the System page reads, counted as the terminal reads and
	// writes: atomics, so that a page sampling them does not hold the
	// reader.
	rx, tx, wireRX, wireTX                         atomic.Uint64
	nopRecv, gaRecv, subneg                        atomic.Uint64
	nWill, nWont, nDo, nDont                       atomic.Uint64
	sentWill, sentWont, sentDo, sentDont           atomic.Uint64
	echoOpts, sgaOpts, nopSent                     atomic.Uint64
	badIAC, badSub                                 atomic.Uint64
	readErrs, writeErrs, readTOs, writeTOs, resets atomic.Uint64

	closed atomic.Bool
	// stMu guards the negotiation states, the last activity and the last
	// error.
	stMu    sync.Mutex
	opts    Options
	lastRX  time.Time
	lastTX  time.Time
	lastNOP time.Time
	lastErr string
}

// The reader's states.
const (
	stData byte = iota
	stIAC
	stOpt
	stSub
	stSubIAC
)

// Dial connects to a Telnet server, and answers its negotiations as the
// terminal asks.
func Dial(ctx context.Context, addr string) (*Conn, error) {
	var d net.Dialer
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return New(nc, "xterm-256color"), nil
}

// New is a Telnet connection on nc, asking the server for term as the
// terminal type.
func New(nc net.Conn, term string) *Conn {
	if term == "" {
		term = "xterm-256color"
	}
	c := &Conn{Conn: nc, term: term, subOpt: -1}
	c.r = bufio.NewReader(&countingReader{r: nc, n: &c.wireRX})
	return c
}

// countingReader counts the bytes the socket carried, before the
// negotiations are taken out.
type countingReader struct {
	r io.Reader
	n *atomic.Uint64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n.Add(uint64(n))
	}
	return n, err
}

// Read reads what the server sent, the negotiations taken out of it. It
// returns once the bytes that have come are read, rather than filling p:
// a prompt is a few bytes of a server that then waits.
func (c *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := 0
	for {
		if n > 0 && c.r.Buffered() == 0 {
			c.noteRead(n)
			return n, nil
		}
		b, data, err := c.next()
		if err != nil {
			if n > 0 {
				c.noteRead(n)
				return n, nil
			}
			c.noteErr(err, true)
			return 0, err
		}
		if data {
			p[n] = b
			n++
			if n == len(p) {
				c.noteRead(n)
				return n, nil
			}
		}
	}
}

// next reads one byte of the connection: a byte of the terminal's data, or
// one of a command, which it answers.
func (c *Conn) next() (b byte, data bool, err error) {
	for {
		ch, err := c.r.ReadByte()
		if err != nil {
			return 0, false, err
		}
		switch c.state {
		case stData:
			if ch == iac {
				c.state = stIAC
				continue
			}
			return ch, true, nil
		case stIAC:
			switch ch {
			case iac:
				c.state = stData
				return iac, true, nil
			case do:
				c.nDo.Add(1)
				c.verb, c.state = ch, stOpt
			case dont:
				c.nDont.Add(1)
				c.verb, c.state = ch, stOpt
			case will:
				c.nWill.Add(1)
				c.verb, c.state = ch, stOpt
			case wont:
				c.nWont.Add(1)
				c.verb, c.state = ch, stOpt
			case sb:
				c.subneg.Add(1)
				c.subOpt, c.sub, c.state = -1, c.sub[:0], stSub
			case se:
				c.state = stData
			case nop:
				c.nopRecv.Add(1)
				c.state = stData
			case ga:
				c.gaRecv.Add(1)
				c.state = stData
			default:
				if ch < se { // 0-239: no command the protocol has
					c.badIAC.Add(1)
				}
				c.state = stData // a command of no option
			}
		case stOpt:
			c.state = stData
			c.negotiate(c.verb, ch)
		case stSub:
			if ch == iac {
				c.state = stSubIAC
				continue
			}
			if c.subOpt < 0 {
				c.subOpt = int(ch)
			} else {
				c.sub = append(c.sub, ch)
			}
		case stSubIAC:
			switch ch {
			case se:
				c.state = stData
				c.subnegotiate(c.subOpt, c.sub)
			case iac:
				c.sub = append(c.sub, iac)
				c.state = stSub
			default:
				// A subnegotiation holds no command but SE: read on,
				// and count the malformed one for the page.
				c.badSub.Add(1)
				c.state = stSub
			}
		}
	}
}

// negotiate answers a request of the server: the options the terminal
// keeps, and the ones it does not.
func (c *Conn) negotiate(verb, opt byte) {
	switch opt {
	case optEcho:
		c.echoOpts.Add(1)
	case optSGA:
		c.sgaOpts.Add(1)
	}
	switch verb {
	case will:
		if opt == optEcho || opt == optSGA || opt == optBinary {
			c.command(do, opt)
			c.setOption(opt, OptionOn)
		} else {
			c.command(dont, opt)
			c.setOption(opt, OptionRefused)
		}
	case do:
		switch opt {
		case optSGA, optBinary, optTTYPE, optNAWS:
			c.command(will, opt)
			if opt == optNAWS {
				c.sendSize()
			}
			if opt == optSGA {
				c.setOption(opt, OptionOn)
			}
		default:
			c.command(wont, opt)
			c.setOption(opt, OptionRefused)
		}
	}
}

// setOption keeps what the client answered an option the page shows.
func (c *Conn) setOption(opt byte, state Option) {
	c.stMu.Lock()
	switch opt {
	case optEcho:
		c.opts.Echo = state
	case optSGA:
		c.opts.SGA = state
	case optLinemode:
		c.opts.Linemode = state
	}
	c.stMu.Unlock()
}

// subnegotiate answers the subnegotiations the client has an answer to:
// the terminal type, which a server asks for by name.
func (c *Conn) subnegotiate(opt int, data []byte) {
	if opt < 0 {
		c.badSub.Add(1) // a subnegotiation with no option at all
		return
	}
	if opt == optTTYPE && len(data) > 0 && data[0] == ttypeSend {
		reply := append([]byte{ttypeIs}, c.term...)
		c.subcommand(byte(opt), reply)
	}
}

// command writes a negotiation.
func (c *Conn) command(verb, opt byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.writeWire([]byte{iac, verb, opt}); err != nil {
		return
	}
	switch verb {
	case will:
		c.sentWill.Add(1)
	case wont:
		c.sentWont.Add(1)
	case do:
		c.sentDo.Add(1)
	case dont:
		c.sentDont.Add(1)
	}
}

// subcommand writes a subnegotiation of opt.
func (c *Conn) subcommand(opt byte, data []byte) {
	buf := make([]byte, 0, len(data)+5)
	buf = append(buf, iac, sb, opt)
	for _, b := range data {
		buf = append(buf, b)
		if b == iac {
			buf = append(buf, iac)
		}
	}
	buf = append(buf, iac, se)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeWire(buf)
}

// writeWire writes buf to the socket, counting its bytes and a failure;
// the caller holds mu.
func (c *Conn) writeWire(buf []byte) (int, error) {
	n, err := c.Conn.Write(buf)
	if n > 0 {
		c.wireTX.Add(uint64(n))
		c.stMu.Lock()
		c.lastTX = time.Now()
		c.stMu.Unlock()
	}
	if err != nil {
		c.noteErr(err, false)
	}
	return n, err
}

// Resize tells the server the size of the terminal's screen, as NAWS has
// it: the client offers the option, and sends the size.
func (c *Conn) Resize(cols, rows int) error {
	c.mu.Lock()
	c.cols, c.rows = cols, rows
	offer := !c.naws
	c.naws = true
	c.mu.Unlock()
	if offer {
		c.command(will, optNAWS)
	}
	c.sendSize()
	return nil
}

// sendSize writes the window's size: two bytes each, the columns then the
// rows.
func (c *Conn) sendSize() {
	c.mu.Lock()
	cols, rows := c.cols, c.rows
	c.mu.Unlock()
	if cols <= 0 || rows <= 0 {
		return
	}
	size := make([]byte, 4)
	binary.BigEndian.PutUint16(size[0:2], uint16(cols))
	binary.BigEndian.PutUint16(size[2:4], uint16(rows))
	c.subcommand(optNAWS, size)
}

// Write writes what the terminal typed, doubling 0xFF, which the protocol
// takes as its own command.
func (c *Conn) Write(p []byte) (int, error) {
	buf := make([]byte, 0, len(p))
	for _, b := range p {
		buf = append(buf, b)
		if b == iac {
			buf = append(buf, iac)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.writeWire(buf); err != nil {
		return 0, err
	}
	c.tx.Add(uint64(len(p)))
	return len(p), nil
}

// SendNOP writes the protocol's NOP, which the System page sends as a
// heartbeat. RFC 854 has the server answer nothing, so the page counts a
// heartbeat answered when the socket carries any byte after it.
func (c *Conn) SendNOP() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.writeWire([]byte{iac, nop}); err != nil {
		return err
	}
	c.nopSent.Add(1)
	c.stMu.Lock()
	c.lastNOP = time.Now()
	c.stMu.Unlock()
	return nil
}

// Close closes the connection, ending the terminal that reads it.
func (c *Conn) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

// noteRead counts the bytes the terminal took.
func (c *Conn) noteRead(n int) {
	if n <= 0 {
		return
	}
	c.rx.Add(uint64(n))
	c.stMu.Lock()
	c.lastRX = time.Now()
	c.stMu.Unlock()
}

// noteErr counts a failure of the socket: a timeout, a reset, or the rest.
func (c *Conn) noteErr(err error, read bool) {
	if err == nil || c.closed.Load() {
		return
	}
	timeout := false
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		timeout = true
	}
	switch {
	case timeout && read:
		c.readTOs.Add(1)
	case timeout:
		c.writeTOs.Add(1)
	case read:
		c.readErrs.Add(1)
	default:
		c.writeErrs.Add(1)
	}
	if isReset(err) {
		c.resets.Add(1)
	}
	c.stMu.Lock()
	c.lastErr = err.Error()
	c.stMu.Unlock()
}

// isReset tells whether err is the far side's RST.
func isReset(err error) bool {
	return err != nil && (errors.Is(err, syscall.ECONNRESET) ||
		strings.Contains(strings.ToLower(err.Error()), "connection reset"))
}

// Stats is what the connection has carried and seen so far.
func (c *Conn) Stats() Stats {
	st := Stats{
		RX: c.rx.Load(), TX: c.tx.Load(),
		WireRX: c.wireRX.Load(), WireTX: c.wireTX.Load(),
		NOP: c.nopRecv.Load(), GA: c.gaRecv.Load(), Subneg: c.subneg.Load(),
		WILL: c.nWill.Load(), WONT: c.nWont.Load(), DO: c.nDo.Load(), DONT: c.nDont.Load(),
		SentWILL: c.sentWill.Load(), SentWONT: c.sentWont.Load(),
		SentDO: c.sentDo.Load(), SentDONT: c.sentDont.Load(),
		Echo: c.echoOpts.Load(), SGA: c.sgaOpts.Load(), NOPsSent: c.nopSent.Load(),
		InvalidIAC: c.badIAC.Load(), Malformed: c.badSub.Load(),
		ReadErrs: c.readErrs.Load(), WriteErrs: c.writeErrs.Load(),
		ReadTimeouts: c.readTOs.Load(), WriteTimeouts: c.writeTOs.Load(),
		Resets: c.resets.Load(),
		Closed: c.closed.Load(),
	}
	c.stMu.Lock()
	st.LastRX, st.LastTX, st.LastNOP = c.lastRX, c.lastTX, c.lastNOP
	st.LastErr = c.lastErr
	c.stMu.Unlock()
	return st
}

// Options is what the client answered the server's negotiations.
func (c *Conn) Options() Options {
	c.stMu.Lock()
	defer c.stMu.Unlock()
	return c.opts
}

// Activity is the last time the socket carried a byte in either direction.
func (c *Conn) Activity() time.Time {
	c.stMu.Lock()
	defer c.stMu.Unlock()
	if c.lastRX.After(c.lastTX) {
		return c.lastRX
	}
	return c.lastTX
}
