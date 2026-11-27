package app

import (
	"bytes"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/serial"
	"finallyshell/internal/telnet"
	"finallyshell/locales"

	gserial "go.bug.st/serial"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// pinPort is a port that reports the input pins a test sets: the rest of
// the library's port is never asked for.
type pinPort struct {
	serial.Port
	bits gserial.ModemStatusBits
}

func (p *pinPort) GetModemStatusBits() (*gserial.ModemStatusBits, error) {
	b := p.bits
	return &b, nil
}

// rawApp is the app with the terminal plugin loaded: a serial port and a
// Telnet server both open one.
func rawApp(t *testing.T) (*App, *fakePlatform, *ui.Tester) {
	t.Helper()
	if err := terminal.Load(); err != nil {
		t.Skip("no terminal:", err)
	}
	a, p, tt, _ := startApp(t)
	t.Cleanup(a.history.pending.Wait)
	return a, p, tt
}

// readUntil collects what the other end of a connection read, until it has
// want.
func readUntil(t *testing.T, rx <-chan string, want string) string {
	t.Helper()
	var all strings.Builder
	deadline := time.After(5 * time.Second)
	for !strings.Contains(all.String(), want) {
		select {
		case chunk := <-rx:
			all.WriteString(chunk)
		case <-deadline:
			t.Fatalf("the other end read %q, not %q", all.String(), want)
		}
	}
	return all.String()
}

// A Telnet host connects to its server, its terminal reads what the server
// sends once the negotiations are taken out, what is typed goes back, and
// the console bar's line goes back with CR LF.
func TestTelnetSession(t *testing.T) {
	a, p, tt := rawApp(t)
	oldPoll := telnetMeterPoll
	telnetMeterPoll = 20 * time.Millisecond
	t.Cleanup(func() { telnetMeterPoll = oldPoll })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	rx := make(chan string, 32)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		// DO SGA, then the prompt, as a server asks and says.
		c.Write([]byte{255, 253, 3})
		c.Write([]byte("Router> "))
		// The answers to the negotiations come before what is typed.
		buf := make([]byte, 128)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				rx <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	h := &config.Host{Name: "router", Addr: "127.0.0.1", Port: port, Protocol: config.ProtocolTelnet}
	a.hosts.Add(h)
	a.setLeftPage(leftSessions) // a connection brings the System page up
	a.openSessionIn(nil, h)
	s := a.current()
	if s == nil || !s.isTelnet() || !s.isRaw() {
		t.Fatalf("the session %v", s)
	}
	p.until(t, "the connection", func() bool { return s.status != statusConnecting })
	if s.status == statusFailed {
		t.Fatalf("the connection failed: %v", s.err)
	}
	untilScreen(t, p, s, "Router>")
	if s.status != statusConnected || s.err != nil {
		t.Fatalf("status %v, err %v", s.status, s.err)
	}
	if s.client != nil {
		t.Error("a Telnet session has an SSH client")
	}
	if a.left.page != leftSystem {
		t.Errorf("the left panel stayed on %d", a.left.page)
	}
	tt.Frame()
	tt.Type("show version")
	tt.Key(0, ui.KeyEnter)
	if got := readUntil(t, rx, "show version"); !strings.Contains(got, "show version") {
		t.Errorf("the server read %q", got)
	}
	// The console bar sends the line with Telnet's CR LF.
	if !tt.HasText(locales.RawSend()) || !tt.HasText(locales.RawClear()) {
		t.Fatalf("no console bar; texts %q", tt.Texts())
	}
	typeInto(t, tt, locales.RawInputLabel(), "status")
	tt.Key(0, ui.KeyEnter)
	if got := readUntil(t, rx, "status"); !strings.HasSuffix(got, "status\r\n") {
		t.Errorf("the bar sent %q", got)
	}
	if why := a.noBottomPanel(); why == "" {
		t.Error("a Telnet session has a bottom panel")
	}
	p.until(t, "the page's options", func() bool {
		return a.telnetMeter(s).view().Opts.SGA == telnet.OptionOn
	})
	tt.Frame()
	if !tt.HasText("127.0.0.1:" + strconv.Itoa(port)) {
		t.Errorf("the side panel; texts %q", tt.Texts())
	}
	// The page is the Telnet one: the link and its options, the traffic,
	// the socket's errors and the IAC commands.
	for _, title := range []string{locales.TelnetLink(), locales.SerialTraffic(),
		locales.TelnetNetErrors(), locales.TelnetProto()} {
		if !tt.HasText(title) {
			t.Errorf("no %q in the Telnet page; texts %q", title, tt.Texts())
		}
	}
	if !tt.HasText("SGA") || !tt.HasText(locales.TelnetAgreed()) {
		t.Errorf("the negotiated options are not shown; texts %q", tt.Texts())
	}
	if !tt.HasText(locales.TelnetHeartbeat()) || !tt.HasText(locales.TelnetState()) {
		t.Errorf("the link rows are missing; texts %q", tt.Texts())
	}
	saveShot(t, tt, "telnet-session-en.png")
}

// A serial host opens its port as the line's settings say, its terminal
// reads what the device sends, the console bar writes a line with CR, the
// timestamps' switch stamps each line, and Clear empties the screen.
func TestSerialSession(t *testing.T) {
	a, p, tt := rawApp(t)
	oldPoll := serialMeterPoll
	serialMeterPoll = 20 * time.Millisecond
	t.Cleanup(func() { serialMeterPoll = oldPoll })
	client, device := net.Pipe()
	defer client.Close()
	defer device.Close()
	old := openSerialPort
	t.Cleanup(func() { openSerialPort = old })
	var opened serial.Config
	openSerialPort = func(c serial.Config) (io.ReadWriteCloser, error) {
		opened = c
		return client, nil
	}
	rx := make(chan string, 64)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := device.Read(buf)
			if n > 0 {
				rx <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go device.Write([]byte("switch# "))

	h := &config.Host{Name: "console", Protocol: config.ProtocolSerial, Serial: &config.SerialSettings{
		Port: "COM3", Baud: 9600, DataBits: 8, Parity: "none", StopBits: "1",
	}}
	a.hosts.Add(h)
	a.setLeftPage(leftSessions) // a connection brings the System page up
	a.openSessionIn(nil, h)
	s := a.current()
	if s == nil || !s.isSerial() {
		t.Fatalf("the session %v", s)
	}
	p.until(t, "the connection", func() bool { return s.status != statusConnecting })
	if s.status == statusFailed {
		t.Fatalf("the connection failed: %v", s.err)
	}
	untilScreen(t, p, s, "switch#")
	if opened.Port != "COM3" || opened.Baud != 9600 || opened.DataBits != 8 ||
		opened.Parity != "none" || opened.StopBits != "1" {
		t.Errorf("the port opened as %+v", opened)
	}
	// Typing into the terminal goes to the port.
	tt.Frame()
	tt.Type("enable")
	tt.Key(0, ui.KeyEnter)
	if got := readUntil(t, rx, "enable"); !strings.Contains(got, "enable") {
		t.Errorf("the port read %q", got)
	}
	// The console bar sends its line with a CR, as a console takes Enter.
	if !tt.HasText(locales.RawSend()) || !tt.HasText(locales.RawClear()) {
		t.Fatalf("no console bar; texts %q", tt.Texts())
	}
	typeInto(t, tt, locales.RawInputLabel(), "show run")
	tt.Key(0, ui.KeyEnter)
	if got := readUntil(t, rx, "show run"); !strings.HasSuffix(got, "show run\r") {
		t.Errorf("the bar sent %q", got)
	}
	// The next line is typed at once: the input takes the keyboard back.
	tt.Frame()
	if !tt.Focused(locales.RawInputLabel()) {
		t.Error("the input lost the keyboard after a send")
	}
	// Stamped lines begin with the time they came.
	if err := tt.Click(locales.RawTimestamps()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// The line the prompt is on began before the switch went on: the
	// stamped ones begin after it.
	go device.Write([]byte("\r\nbooting\r\nready\r\nError: boom\r\n"))
	stamped := regexp.MustCompile(`【\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}】`)
	p.until(t, "the stamped lines", func() bool {
		return stamped.MatchString(s.term().Text()) &&
			strings.Contains(s.term().Text(), "】booting") &&
			strings.Contains(s.term().Text(), "】ready")
	})
	// The System page shows the link, what the port carried, the errors,
	// the buffers, the timing and the lines, and the connection brought
	// it up by itself.
	if a.left.page != leftSystem {
		t.Errorf("the left panel stayed on %d", a.left.page)
	}
	p.until(t, "the page's totals", func() bool {
		v := a.serialMeter(s).view()
		return v.RX > 0 && v.TX > 0
	})
	tt.Frame()
	for _, title := range []string{locales.SerialLink(), locales.SerialTraffic(),
		locales.SerialErrors(), locales.SerialBuffers(), locales.SerialTiming(), locales.SerialParse()} {
		if !tt.HasText(title) {
			t.Errorf("no %q in the serial page; texts %q", title, tt.Texts())
		}
	}
	if !tt.HasText("DTR "+locales.SerialHigh()) || !tt.HasText("RTS "+locales.SerialHigh()) {
		t.Errorf("the pins are not shown; texts %q", tt.Texts())
	}
	// A net.Pipe is no serial.Port: the driver's counts are unsupported,
	// and the page says so rather than show zeroes.
	if !tt.HasText(locales.SerialNoCounts()) {
		t.Errorf("no note on the unsupported counts; texts %q", tt.Texts())
	}
	if !tt.HasText("panic 0") || !tt.HasText("error 1") {
		t.Errorf("the keywords are not counted; texts %q", tt.Texts())
	}
	m := a.serialMeter(s)
	v := m.view()
	if v.RX == 0 || v.TX == 0 {
		t.Errorf("the meter has no bytes: %+v", v)
	}
	if !tt.HasText(formatBytes(float64(v.RX))) {
		t.Errorf("the received bytes are not shown; texts %q", tt.Texts())
	}
	if !tt.HasText("COM3") {
		t.Errorf("the port is not shown; texts %q", tt.Texts())
	}
	// A port that reports its input pins shows them lit, and the line's
	// state as the driver sees it.
	s.serial.attach(&pinPort{bits: gserial.ModemStatusBits{CTS: true, DSR: true}})
	s.serial.sample(time.Now())
	tt.Frame()
	if !tt.HasText(locales.SerialCTS()+" "+locales.SerialHigh()) ||
		!tt.HasText(locales.SerialDSR()+" "+locales.SerialHigh()) {
		t.Errorf("the modem pins are not shown; texts %q", tt.Texts())
	}
	if !tt.HasText(locales.SerialLineState()) {
		t.Errorf("no line state; texts %q", tt.Texts())
	}
	saveShot(t, tt, "serial-session-en.png")
	// Clear takes the screen and its scrollback away.
	if err := tt.Click(locales.RawClear()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if txt := s.term().Text(); strings.Contains(txt, "booting") || strings.Contains(txt, "switch#") {
		t.Errorf("after the clear %q", txt)
	}
}

// A device that ends its lines with LF alone gets a terminal whose lines
// all begin at the left, not one that walks right with each line.
func TestSerialLoneLF(t *testing.T) {
	a, p, tt := rawApp(t)
	client, device := net.Pipe()
	defer client.Close()
	defer device.Close()
	old := openSerialPort
	t.Cleanup(func() { openSerialPort = old })
	openSerialPort = func(c serial.Config) (io.ReadWriteCloser, error) { return client, nil }
	go device.Write([]byte("收到数据 ===> hello\n收到数据 ===> nihao\n收到数据 ===> 嘻嘻哈哈\n"))

	h := &config.Host{Name: "esp32", Protocol: config.ProtocolSerial, Serial: &config.SerialSettings{Port: "COM3"}}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	s := a.current()
	p.until(t, "the connection", func() bool { return s.status == statusConnected })
	untilScreen(t, p, s, "嘻嘻哈哈")
	tt.Frame()
	vt := s.term().Snapshot()
	if !bytes.Contains(vt, []byte("hello\r\n收到数据 ===> nihao\r\n收到数据 ===> 嘻嘻哈哈")) {
		t.Errorf("the lone LFs were not made CR LFs: %q", vt)
	}
	saveShot(t, tt, "serial-lone-lf-en.png")
}

// chunkConn is a connection that reads the chunks it was given and writes
// to w; size is what the terminal last told it.
type chunkConn struct {
	chunks [][]byte
	w      io.Writer
	size   [2]int
}

func (c *chunkConn) Resize(cols, rows int) error {
	c.size = [2]int{cols, rows}
	return nil
}

func (c *chunkConn) Read(p []byte) (int, error) {
	if len(c.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.chunks[0])
	c.chunks[0] = c.chunks[0][n:]
	if len(c.chunks[0]) == 0 {
		c.chunks = c.chunks[1:]
	}
	return n, nil
}

func (c *chunkConn) Write(p []byte) (int, error) { return c.w.Write(p) }
func (c *chunkConn) Close() error                { return nil }

// The stamp of a line is put before its first byte, once per line, and
// only from the line after the switch goes on: a line already underway is
// not cut in two by it.
func TestStampConnPrefixesLines(t *testing.T) {
	var stamps atomic.Bool
	conn := &chunkConn{
		chunks: [][]byte{[]byte("abc"), []byte("def\r\ng h i\r\n")},
		w:      io.Discard,
	}
	c := newStampConn(conn, &stamps, false, nil, nil)
	first := make([]byte, 16)
	n, err := c.Read(first)
	if err != nil || string(first[:n]) != "abc" {
		t.Fatalf("the first read %q, %v", first[:n], err)
	}
	stamps.Store(true)
	rest, _ := io.ReadAll(c)
	if !strings.HasPrefix(string(rest), "def\r\n") {
		t.Errorf("the line midway was cut: %q", rest)
	}
	if !regexp.MustCompile(`^def\r\n【[^】]+】g h i\r\n$`).MatchString(string(rest)) {
		t.Errorf("the stamped read %q", rest)
	}
	// The switch off leaves the bytes as they came.
	var off atomic.Bool
	conn = &chunkConn{chunks: [][]byte{[]byte("one\r\ntwo")}, w: io.Discard}
	c = newStampConn(conn, &off, false, nil, nil)
	if got, _ := io.ReadAll(c); string(got) != "one\r\ntwo" {
		t.Errorf("unstamped %q", got)
	}
	// The size the terminal sets goes on to the connection: a Telnet
	// server hears of it as NAWS.
	if err := c.Resize(100, 40); err != nil {
		t.Fatal(err)
	}
	if conn.size != [2]int{100, 40} {
		t.Errorf("the connection heard the size %v", conn.size)
	}
}

// A serial device may end its lines with LF alone: the terminal is given CR
// LF, so that each line begins at the left, and a CR at the end of a read
// still makes the LF of the next a pair.
func TestStampConnLoneLF(t *testing.T) {
	var stamps atomic.Bool
	c := newStampConn(&chunkConn{
		chunks: [][]byte{[]byte("one\ntwo\r"), []byte("\nthree\n")},
		w:      io.Discard,
	}, &stamps, true, nil, nil)
	got, _ := io.ReadAll(c)
	if want := "one\r\ntwo\r\nthree\r\n"; string(got) != want {
		t.Errorf("the serial lines %q, want %q", got, want)
	}
	// A Telnet server ends its own lines: its lone LFs are left alone.
	var off atomic.Bool
	c = newStampConn(&chunkConn{chunks: [][]byte{[]byte("one\ntwo")}, w: io.Discard}, &off, false, nil, nil)
	if got, _ := io.ReadAll(c); string(got) != "one\ntwo" {
		t.Errorf("a Telnet line %q", got)
	}
}

// A port whose far side takes no bytes does not hold the window: what is
// sent is queued, and the send returns.
func TestAsyncConnWriteDoesNotWait(t *testing.T) {
	release := make(chan struct{})
	conn := newAsyncConn(&blockingConn{reads: strings.NewReader(""), block: release})
	defer close(release)
	wrote := make(chan error, 1)
	go func() {
		_, err := conn.Write([]byte("hello"))
		wrote <- err
	}()
	select {
	case err := <-wrote:
		if err != nil {
			t.Errorf("the write: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the write waited for the port")
	}
	conn.Close()
}

// blockingConn is a connection whose far side takes no bytes: its writes
// wait for block to close.
type blockingConn struct {
	reads io.Reader
	block chan struct{}
}

func (c *blockingConn) Read(p []byte) (int, error)  { return c.reads.Read(p) }
func (c *blockingConn) Write(p []byte) (int, error) { <-c.block; return len(p), nil }
func (c *blockingConn) Close() error                { return nil }

// trackedConn is a connection that says when it was closed: a serial port
// and a Telnet socket must be released, not left open under a screen that
// stays.
type trackedConn struct {
	net.Conn
	once   sync.Once
	closed chan struct{}
}

func (c *trackedConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// "Disconnect" really lets the port go: the serial port is closed, the
// pane has no wire left, and the driver's pins are no longer read.
func TestSerialDisconnectClosesPort(t *testing.T) {
	a, p, _ := rawApp(t)
	client, device := net.Pipe()
	t.Cleanup(func() { device.Close() })
	port := &trackedConn{Conn: client, closed: make(chan struct{})}
	old := openSerialPort
	t.Cleanup(func() { openSerialPort = old })
	openSerialPort = func(serial.Config) (io.ReadWriteCloser, error) { return port, nil }
	go device.Write([]byte("ready\r\n"))

	h := &config.Host{Name: "board", Protocol: config.ProtocolSerial,
		Serial: &config.SerialSettings{Port: "COM3"}}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	s := a.current()
	p.until(t, "the connection", func() bool { return s.status == statusConnected })
	untilScreen(t, p, s, "ready")

	a.disconnectSession(s)
	select {
	case <-port.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("the serial port stayed open")
	}
	if s.status != statusClosed {
		t.Errorf("the session is %v", s.status)
	}
	for _, pane := range s.panes.panes() {
		if pane.wire != nil {
			t.Error("the pane still writes through the closed port")
		}
	}
	if v := a.serialMeter(s).view(); v.BitsOK || v.QueuesOK || v.ErrsOK {
		t.Errorf("the page still reads the closed port: %+v", v)
	}
}

// A Telnet socket is closed too, and connecting again opens a new one
// rather than leaving the old open.
func TestTelnetDisconnectClosesSocket(t *testing.T) {
	a, p, _ := rawApp(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	closed := make(chan struct{}, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.Write([]byte("prompt> "))
				buf := make([]byte, 64)
				for {
					if _, err := c.Read(buf); err != nil {
						closed <- struct{}{}
						return
					}
				}
			}(c)
		}
	}()
	h := &config.Host{Name: "router", Addr: "127.0.0.1",
		Port: ln.Addr().(*net.TCPAddr).Port, Protocol: config.ProtocolTelnet}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	s := a.current()
	p.until(t, "the connection", func() bool { return s.status == statusConnected })
	untilScreen(t, p, s, "prompt>")

	a.disconnectSession(s)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("the Telnet socket stayed open")
	}
	if m := a.telnetMeter(s); m.conn != nil {
		t.Error("the meter still reads the closed socket")
	}

	a.reconnect(s)
	p.until(t, "the second connection", func() bool { return s.status == statusConnected })
	untilScreen(t, p, s, "prompt>")
	if v := a.telnetMeter(s).view(); v.Reconnects < 1 {
		t.Errorf("reconnects %d", v.Reconnects)
	}
}

// quietConn is a connection whose far side says nothing, which the
// heartbeat of a Telnet page is judged against.
type quietConn struct {
	once   sync.Once
	closed chan struct{}
}

func (c *quietConn) Read([]byte) (int, error)    { <-c.closed; return 0, io.EOF }
func (c *quietConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *quietConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}
func (c *quietConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *quietConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *quietConn) SetDeadline(time.Time) error      { return nil }
func (c *quietConn) SetReadDeadline(time.Time) error  { return nil }
func (c *quietConn) SetWriteDeadline(time.Time) error { return nil }

// The page sends Telnet's NOP as a heartbeat, and a socket that stays
// quiet through several of them is judged a half-open connection.
func TestTelnetMeterHeartbeat(t *testing.T) {
	nc := &quietConn{closed: make(chan struct{})}
	conn := telnet.New(nc, "")
	t.Cleanup(func() { conn.Close() })
	m := &telnetMeter{}
	m.attach(conn, "127.0.0.1:23")
	base := time.Now()
	m.sample(base)
	for i := 1; i <= 5; i++ {
		m.sample(base.Add(time.Duration(i) * 40 * time.Second))
	}
	v := m.view()
	if v.Beats == 0 {
		t.Fatal("no heartbeat was sent")
	}
	if v.BeatsOK != 0 {
		t.Errorf("%d heartbeats answered a quiet socket", v.BeatsOK)
	}
	if !v.HalfOpen {
		t.Error("the quiet socket was not judged half open")
	}
	if v.HalfOpens != 1 {
		t.Errorf("half open counted %d times", v.HalfOpens)
	}
	if v.ReadTimeouts == 0 {
		t.Error("no read timeout counted")
	}
	// The snapshot is taken before the sample's own NOP is written.
	if v.ST.NOPsSent+1 < v.Beats {
		t.Errorf("the connection sent %d NOPs, the page %d", v.ST.NOPsSent, v.Beats)
	}
}
