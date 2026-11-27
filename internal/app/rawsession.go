package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"finallyshell/internal/serial"
	"finallyshell/internal/telnet"
	"finallyshell/locales"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// openSerialPort is serial.Open, which a test replaces with a port of its
// own: a COM port cannot be opened twice, nor on a builder.
var openSerialPort = serial.Open

// connectSerial connects s to a device on a serial port: its terminal is
// the port, and what the device answers is what it shows.
func (a *App) connectSerial(s *Session) { a.connectRaw(s) }

// connectTelnet connects s to a Telnet server: the terminal is the
// connection, once its negotiations are answered (internal/telnet).
func (a *App) connectTelnet(s *Session) { a.connectRaw(s) }

// dialRaw is a connection of s's protocol: the port of a serial host, or
// a Telnet server's socket.
func (a *App) dialRaw(ctx context.Context, s *Session) (io.ReadWriteCloser, error) {
	h := s.Host
	if h.IsSerial() {
		c := h.SerialPort()
		conn, err := openSerialPort(serial.Config{
			Port: c.Port, Baud: c.Baud, DataBits: c.DataBits,
			Parity: c.Parity, StopBits: c.StopBits,
		})
		if err != nil {
			return nil, err
		}
		if port, ok := conn.(serial.Port); ok {
			a.serialMeter(s).attach(port)
		}
		return conn, nil
	}
	if h.IsTelnet() {
		addr := h.AddrPort(23)
		m := a.telnetMeter(s)
		ctx, cancel := context.WithTimeout(ctx, telnetDialTimeout)
		defer cancel()
		var d net.Dialer
		nc, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			m.noteDial(err)
			return nil, err
		}
		conn := telnet.New(nc, h.TermTypeOrDefault())
		m.attach(conn, addr)
		return conn, nil
	}
	return nil, fmt.Errorf("no raw connection for %q", h.Protocol)
}

// rawConsole is what a serial or Telnet tab keeps for its console bar: the
// line waiting to be sent, and whether each line the device sends is
// stamped with the time it came.
type rawConsole struct {
	input  string
	stamps atomic.Bool
	// focus has the input take the keyboard in the next frame: after a
	// send, so that the next line can be typed at once.
	focus bool
}

// rawConsole is s's console bar's state, made the first time it is asked
// for.
func (s *Session) rawConsole() *rawConsole {
	if s.raw == nil {
		s.raw = &rawConsole{}
	}
	return s.raw
}

// stampConn prepares what the device sends for the terminal: a line of the
// device's may be stamped with the time it came, as the console bar's
// switch asks, and a lone LF is a new line — CR LF — rather than the line
// that walks right down the screen.
type stampConn struct {
	io.ReadWriteCloser
	stamps *atomic.Bool
	lfCR   bool   // a lone LF becomes CR LF
	prev   byte   // the last byte of the device's
	buf    []byte // what a read of the connection takes
	held   []byte // what of it was not given to the reader
	line   bool   // the next byte of the device's begins a line
	// onRead is what the device sent, before the stamps; onErr is a read
	// the port failed. Both feed the serial page's meter (serialmeter.go),
	// and are nil for a Telnet server's.
	onRead func([]byte)
	onErr  func()
}

// newStampConn is conn, stamped by the console bar's switch and, when
// lfCR, with its lone LFs turned into CR LFs: a serial device's log is a
// line at a time.
func newStampConn(conn io.ReadWriteCloser, stamps *atomic.Bool, lfCR bool, onRead func([]byte), onErr func()) *stampConn {
	return &stampConn{ReadWriteCloser: conn, stamps: stamps, lfCR: lfCR, line: true,
		onRead: onRead, onErr: onErr}
}

// Resize passes the terminal's size on, as the terminal tells the
// connection it reads: a Telnet server hears of it as NAWS.
func (c *stampConn) Resize(cols, rows int) error {
	if r, ok := c.ReadWriteCloser.(interface {
		Resize(cols, rows int) error
	}); ok {
		return r.Resize(cols, rows)
	}
	return nil
}

// Read reads what the device sent, a stamped line at a time.
func (c *stampConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(c.held) == 0 {
		if c.buf == nil {
			c.buf = make([]byte, 32<<10)
		}
		n, err := c.ReadWriteCloser.Read(c.buf)
		if n == 0 {
			if err != nil && c.onErr != nil {
				c.onErr()
			}
			return 0, err
		}
		if c.onRead != nil {
			c.onRead(c.buf[:n])
		}
		c.held = c.stamp(c.buf[:n])
	}
	n := copy(p, c.held)
	c.held = c.held[n:]
	return n, nil
}

// stamp is data with a line's time put before the first byte of each line,
// when the switch is on, and its lone LFs made CR LFs.
func (c *stampConn) stamp(data []byte) []byte {
	out := make([]byte, 0, len(data)+64)
	for _, b := range data {
		if c.line && b != '\r' && b != '\n' && c.stamps.Load() {
			out = append(out, stampPrefix(time.Now())...)
		}
		if c.lfCR && b == '\n' && c.prev != '\r' {
			out = append(out, '\r')
		}
		out = append(out, b)
		c.prev = b
		c.line = b == '\r' || b == '\n'
	}
	return out
}

// stampPrefix is how a stamped line begins: 【2026-11-26 18:07:39.100】.
func stampPrefix(at time.Time) string {
	return "【" + at.Format("2006-01-02 15:04:05.000") + "】"
}

// connectRaw connects s, with a connection of its own for each of the
// session's panes, as the shells of an SSH session are: the terminals take
// the connections as their own streams, with no shell to follow.
func (a *App) connectRaw(s *Session) {
	dial := func() (io.ReadWriteCloser, error) { return a.dialRaw(s.ctx, s) }
	console := s.rawConsole()
	if s.cancel != nil {
		s.cancel()
	}
	s.status, s.err = statusConnecting, nil
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.ctx = cancel, ctx
	if s.isSerial() {
		a.serialMeter(s) // here, so the meter is ready for the panes
	}
	if s.isTelnet() {
		a.telnetMeter(s).noteAttempt()
	}
	spec := a.shellSpec(s.Host) // here, on the UI thread
	if s.panes.root == nil {
		s.panes.reset(newPane())
	}
	panes := s.panes.panes()
	fail := func(err error) {
		a.post(func() {
			if ctx.Err() != nil {
				return
			}
			s.status, s.err = statusFailed, err
			a.assetLogin(false)
		})
	}
	go func() {
		terms := make([]*terminal.Terminal, len(panes))
		closeTerms := func() {
			for _, t := range terms {
				if t != nil {
					t.Close()
				}
			}
		}
		for i, p := range panes {
			conn, err := dial()
			if err != nil {
				closeTerms()
				fail(err)
				return
			}
			term, err := a.openRawTerminal(s, p, conn, console, spec.topts)
			if err != nil {
				conn.Close()
				closeTerms()
				fail(err)
				return
			}
			terms[i] = term
		}
		a.post(func() {
			if ctx.Err() != nil {
				closeTerms()
				return
			}
			for i, p := range panes {
				switch {
				case terms[i] == nil:
					s.panes.remove(p)
					continue
				case !s.panes.has(p): // closed while connecting
					terms[i].Close()
					continue
				}
				if p.term != nil {
					p.term.Close()
				}
				p.term = terms[i]
				s.panes.opened(p)
				a.updateTerminal(p.term, spec.topts)
			}
			s.panes.refocus = true
			s.status, s.err = statusConnected, nil
			a.assetLogin(true)
			a.noteConnected(s.Host)
			a.autoRecord(s)
			// The System page is what a port or a Telnet server has to
			// show: the left column opens on it as the connection comes
			// up, whichever page it was on.
			a.setLeftPage(leftSystem)
			if s.isSerial() {
				a.startSerialMeter(s, ctx)
			}
			if s.isTelnet() {
				a.startTelnetMeter(s, ctx)
			}
		})
	}()
}

// asyncConn writes on a goroutine of its own, so that a port whose far
// side has stopped taking bytes — a device with no receiver on its line,
// or one holding the flow off — cannot hold the window: what is typed is
// queued, and a queue that fills ends the connection rather than the
// program.
type asyncConn struct {
	io.ReadWriteCloser
	writes chan []byte
	done   chan struct{}
	once   sync.Once
	// What landed, and what the queue could not keep, for the serial
	// page's meter (serialmeter.go); nil for a Telnet server's.
	onWrite func(int)
	onDrop  func()
	// pending is what is queued and not yet written.
	pendingChunks atomic.Int64
	pendingBytes  atomic.Int64
}

// Pending is what has been queued for the port and not yet written: the
// chunks and their bytes.
func (c *asyncConn) Pending() (chunks, bytes int) {
	return int(c.pendingChunks.Load()), int(c.pendingBytes.Load())
}

// newAsyncConn is conn, written to in the background.
func newAsyncConn(conn io.ReadWriteCloser) *asyncConn {
	c := &asyncConn{
		ReadWriteCloser: conn,
		writes:          make(chan []byte, 256),
		done:            make(chan struct{}),
	}
	go c.pump()
	return c
}

// pump writes what Write queued, in order, and ends the connection when a
// write fails.
func (c *asyncConn) pump() {
	for {
		select {
		case <-c.done:
			return
		case b := <-c.writes:
			if _, err := c.ReadWriteCloser.Write(b); err != nil {
				c.Close()
				return
			}
			c.pendingChunks.Add(-1)
			c.pendingBytes.Add(-int64(len(b)))
			if c.onWrite != nil {
				c.onWrite(len(b))
			}
		}
	}
}

// Write queues p and returns: the window does not wait for the port.
func (c *asyncConn) Write(p []byte) (int, error) {
	b := append([]byte(nil), p...)
	select {
	case c.writes <- b:
		c.pendingChunks.Add(1)
		c.pendingBytes.Add(int64(len(b)))
		return len(p), nil
	case <-c.done:
		return 0, io.ErrClosedPipe
	default:
		// The queue is full: the far side has not taken what it was
		// given. Ending the connection shows that, rather than freezing
		// the window until it does.
		if c.onDrop != nil {
			c.onDrop()
		}
		c.Close()
		return 0, io.ErrShortWrite
	}
}

func (c *asyncConn) Close() error {
	var err error
	c.once.Do(func() {
		close(c.done)
		// What was queued is not going anywhere: the page's buffer line
		// must not keep counting it.
		c.pendingChunks.Store(0)
		c.pendingBytes.Store(0)
		err = c.ReadWriteCloser.Close()
	})
	return err
}

// Resize passes the terminal's size on, as the terminal tells the
// connection it reads.
func (c *asyncConn) Resize(cols, rows int) error {
	if r, ok := c.ReadWriteCloser.(interface {
		Resize(cols, rows int) error
	}); ok {
		return r.Resize(cols, rows)
	}
	return nil
}

// openRawTerminal is the terminal of p over conn: the connection is its
// stream, stamped by console's switch, and no shell of an SSH server
// stands behind it.
func (a *App) openRawTerminal(s *Session, p *pane, conn io.ReadWriteCloser, console *rawConsole, topts terminal.Options) (*terminal.Terminal, error) {
	serial := s.Host != nil && s.Host.IsSerial()
	meter := s.serial
	async := newAsyncConn(conn)
	var onRead func([]byte)
	var onErr func()
	if serial && meter != nil {
		async.onWrite, async.onDrop = meter.noteWrite, meter.noteDrop
		meter.addWire(async)
		onRead, onErr = meter.noteRead, meter.noteReadErr
	}
	held := make(chan struct{})
	opts := topts
	wire := &heldConn{
		ReadWriteCloser: &recordConn{ReadWriteCloser: async, app: a, s: s},
		ready:           held,
	}
	opts.Conn = newStampConn(wire, &console.stamps, serial, onRead, onErr)
	opts.NoExitMessage = true
	opts.OnKey = func(mods ui.Modifiers, key ui.Key) bool {
		a.assetKey(s, mods, key)
		return false
	}
	opts.OnExit = func(int) { a.post(func() { a.paneExited(s, p) }) }
	term, err := terminal.New(opts)
	if err != nil {
		close(held)
		async.Close()
		return nil, err
	}
	p.restore(term)
	p.wire = wire
	close(held)
	return term, nil
}

// rawBar is the console bar under the terminal of a serial or Telnet
// session: a line to send to the device, a clear of the screen, and
// whether each line the device sends is stamped with the time it came.
func (a *App) rawBar(c *ui.Context, s *Session) {
	con := s.rawConsole()
	live := s.status == statusConnected
	ui.Row(c).Background(theme.Background).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).
		Padding(6, 10).Gap(8).AlignItems(ui.Center).Children(func() {
		field := ui.TextInput(c, &con.input).Label(locales.RawInputLabel()).
			Placeholder(locales.RawInputPlaceholder()).Grow(1).MinWidth(0).
			Disabled(!live)
		if con.focus {
			field.AutoFocus()
			con.focus = false
		}
		send := field.Submitted()
		if ui.Button(c, locales.RawSend()).Disabled(!live).Clicked() || send {
			a.rawSend(s, con)
		}
		if ui.Button(c, locales.RawClear()).Disabled(!live).Clicked() {
			if t := s.term(); t != nil {
				t.ClearScreen()
			}
		}
		on := con.stamps.Load()
		if ui.Checkbox(c, &on, locales.RawTimestamps()).Changed() {
			con.stamps.Store(on)
		}
	})
}

// rawSend writes the console bar's line to the device, and empties it: a
// serial console takes CR for Enter, a Telnet server CR LF.
func (a *App) rawSend(s *Session, con *rawConsole) {
	p := s.pane()
	if p == nil || p.wire == nil {
		return
	}
	line := con.input + "\r"
	if s.Host != nil && s.Host.IsTelnet() {
		line = con.input + "\r\n"
	}
	con.input = ""
	con.focus = true
	if _, err := io.WriteString(p.wire, line); err != nil {
		a.notify(describeError(err))
	}
}
