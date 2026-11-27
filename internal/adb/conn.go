package adb

// What a connection carries, its channels and its protocol, as the System
// page of its tab shows them: the ADB stream is a socket per service, and a
// shell, a log stream and a command are all channels of the same
// connection.

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	goadb "github.com/pgaskin/go-adb/adb"
	"github.com/pgaskin/go-adb/adb/adbproto/aproto"
	"github.com/pgaskin/go-adb/adblib/adbsync"
)

// ClientVersion is the ADB protocol version the app announces, as the
// page shows it before a connection's own numbers are there.
const ClientVersion = aproto.VersionMax

// Channel is a kind of ADB channel: the terminal's shell, a command the app
// runs, the log stream, and the kinds the app does not open yet.
type Channel int

const (
	ChannelShell Channel = iota
	ChannelExec
	ChannelLogcat
	ChannelSync
	ChannelJDWP

	channelCount
)

// String is the channel as the page names it.
func (c Channel) String() string {
	switch c {
	case ChannelShell:
		return "shell"
	case ChannelExec:
		return "exec"
	case ChannelLogcat:
		return "logcat"
	case ChannelSync:
		return "sync"
	case ChannelJDWP:
		return "jdwp"
	}
	return "other"
}

// LogcatStats is what the log stream has carried, by level.
type LogcatStats struct {
	Alive            bool
	Lines, Bytes     uint64
	V, D, I, W, E, F uint64
	Drops            uint64
}

// Stats is what a connection has carried and opened, and how it went.
type Stats struct {
	// RX and TX are the bytes of the terminal's stream, and Packets the
	// data packets of the shell protocol that carried them.
	RX, TX  uint64
	Packets uint64

	// The ADB packets the app itself sees: the connection's handshake, and
	// the channels it opens and closes. WRTE packets are counted as the
	// shell protocol's data packets, above.
	CNXN, OPEN, OKAY, CLSE uint64
	// Failures are channels that could not be opened, and Malformed the
	// packets of a stream that broke the shell protocol.
	Failures, Malformed uint64

	// Active channels, and the ones opened by kind over the connection.
	Active          int
	ActiveByChannel [channelCount]int
	Opened          [channelCount]uint64

	// The socket's failures.
	ReadErrs, WriteErrs, Timeouts uint64
	LastRX, LastTX                time.Time
	LastErr                       string
	Closed                        bool

	// ClientVersion is the ADB protocol version the app announces; the
	// device's own is not in its banner, and is read with the device
	// metrics instead.
	ClientVersion uint32

	Logcat LogcatStats
}

// counters is the connection's statistics, counted as its goroutines run.
type counters struct {
	mu sync.Mutex
	st Stats
}

func (c *counters) connected() {
	c.mu.Lock()
	c.st.CNXN++
	c.st.ClientVersion = aproto.VersionMax
	c.mu.Unlock()
}

func (c *counters) open(k Channel) {
	c.mu.Lock()
	c.st.OPEN++
	c.st.OKAY++
	c.st.Active++
	if int(k) < int(channelCount) {
		c.st.ActiveByChannel[k]++
		c.st.Opened[k]++
	}
	c.mu.Unlock()
}

func (c *counters) failed(k Channel) {
	c.mu.Lock()
	c.st.OPEN++
	c.st.Failures++
	c.mu.Unlock()
}

func (c *counters) closed(k Channel) {
	c.mu.Lock()
	c.st.CLSE++
	if c.st.Active > 0 {
		c.st.Active--
	}
	if int(k) < int(channelCount) && c.st.ActiveByChannel[k] > 0 {
		c.st.ActiveByChannel[k]--
	}
	c.mu.Unlock()
}

func (c *counters) read(n int, err error) {
	c.mu.Lock()
	c.st.RX += uint64(n)
	if n > 0 {
		c.st.LastRX = time.Now()
	}
	c.mu.Unlock()
	if err != nil {
		c.err(err, true)
	}
}

func (c *counters) write(n int, err error) {
	c.mu.Lock()
	c.st.TX += uint64(n)
	if n > 0 {
		c.st.LastTX = time.Now()
	}
	c.mu.Unlock()
	if err != nil {
		c.err(err, false)
	}
}

// err counts a failure of the socket, as the page says it.
func (c *counters) err(err error, read bool) {
	if err == nil || errors.Is(err, io.EOF) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var ne interface{ Timeout() bool }
	switch {
	case errors.As(err, &ne) && ne.Timeout():
		c.st.Timeouts++
	case read:
		c.st.ReadErrs++
	default:
		c.st.WriteErrs++
	}
	c.st.LastErr = err.Error()
}

func (c *counters) malformed() {
	c.mu.Lock()
	c.st.Malformed++
	c.mu.Unlock()
}

// packets counts the data packets of the shell protocol, both ways.
func (c *counters) packets(in, out uint64) {
	c.mu.Lock()
	c.st.Packets += in + out
	c.mu.Unlock()
}

func (c *counters) logcat(f func(*LogcatStats)) {
	c.mu.Lock()
	f(&c.st.Logcat)
	c.mu.Unlock()
}

func (c *counters) closedConn() {
	c.mu.Lock()
	c.st.Closed = true
	c.mu.Unlock()
}

// Stats is what the connection has carried so far.
func (c *Conn) Stats() Stats {
	if c == nil || c.count == nil {
		return Stats{}
	}
	c.count.mu.Lock()
	defer c.count.mu.Unlock()
	return c.count.st
}

// Exec runs a command on the device over a channel of its own, and returns
// what it printed. It blocks; run it on a goroutine of its own.
func (c *Conn) Exec(ctx context.Context, command string) (string, error) {
	if c == nil || c.dial == nil {
		return "", errors.New("adb: no connection to run a command on")
	}
	nc, err := goadb.Shell(ctx, c.dial, command)
	if err != nil {
		c.count.failed(ChannelExec)
		return "", err
	}
	c.count.open(ChannelExec)
	defer func() {
		nc.Close()
		c.count.closed(ChannelExec)
	}()
	var b strings.Builder
	buf := make([]byte, 8<<10)
	for {
		n, rerr := nc.Read(buf)
		if n > 0 {
			if b.Len() < 256<<10 {
				b.Write(buf[:n])
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				return b.String(), nil
			}
			return b.String(), rerr
		}
	}
}

// WatchLogcat reads the device's log over a channel of its own, counting
// its lines by level, until ctx ends. It is started once.
func (c *Conn) WatchLogcat(ctx context.Context) {
	if c == nil || c.dial == nil {
		return
	}
	c.logOnce.Do(func() {
		go c.watchLogcat(ctx)
	})
}

func (c *Conn) watchLogcat(ctx context.Context) {
	for ctx.Err() == nil {
		nc, err := goadb.Shell(ctx, c.dial, "logcat -v brief")
		if err != nil {
			c.count.failed(ChannelLogcat)
			if !sleepCtx(ctx, 2*time.Second) {
				return
			}
			continue
		}
		c.count.open(ChannelLogcat)
		c.count.logcat(func(l *LogcatStats) { l.Alive = true })
		c.readLogcat(ctx, nc)
		nc.Close()
		c.count.closed(ChannelLogcat)
		c.count.logcat(func(l *LogcatStats) { l.Alive = false })
		if !sleepCtx(ctx, time.Second) {
			return
		}
	}
}

// readLogcat counts the lines of the stream by level, as "I/Activity( 1):".
func (c *Conn) readLogcat(ctx context.Context, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 32<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		n := len(line)
		c.count.logcat(func(l *LogcatStats) {
			l.Lines++
			l.Bytes += uint64(n)
			if n >= 2 && line[1] == '/' {
				switch line[0] {
				case 'V':
					l.V++
				case 'D':
					l.D++
				case 'I':
					l.I++
				case 'W':
					l.W++
				case 'E':
					l.E++
				case 'F':
					l.F++
				}
			}
		})
		if ctx.Err() != nil {
			return
		}
	}
}

// sleepCtx waits, and tells whether ctx is still open.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Conn is a shell of the device, with the connection it runs on and what
// it has carried.
type Conn struct {
	stream io.ReadWriteCloser
	closer io.Closer
	dial   goadb.Dialer
	count  *counters
	// syncClient is the device's sync service, one client for all the
	// folders the app lists and the files it copies (fs.go): a client of
	// its own would leave its channels open on the device.
	syncClient *adbsync.Client

	closeOnce sync.Once
	logOnce   sync.Once
}

// Read reads what the shell wrote, counting it.
func (c *Conn) Read(p []byte) (int, error) {
	n, err := c.stream.Read(p)
	c.count.read(n, err)
	return n, err
}

// Write writes what the terminal typed, counting it.
func (c *Conn) Write(p []byte) (int, error) {
	n, err := c.stream.Write(p)
	c.count.write(n, err)
	return n, err
}

// Resize tells the device the size of the terminal's window, where the
// shell has the protocol for it.
func (c *Conn) Resize(cols, rows int) error {
	if r, ok := c.stream.(interface{ Resize(int, int) error }); ok {
		return r.Resize(cols, rows)
	}
	return nil
}

// Close closes the shell and the connection it runs on.
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.count.closed(ChannelShell)
		c.count.closedConn()
		err = c.stream.Close()
		if c.closer != nil {
			c.closer.Close()
		}
	})
	return err
}

// packets tells the connection's counters of a stream's packets, which the
// shell protocol counts as the transport's data packets.
type packetCounter struct {
	count *counters
	in    uint64
	out   uint64
	mu    sync.Mutex
}

func (p *packetCounter) sent() {
	p.mu.Lock()
	p.out++
	p.mu.Unlock()
}

func (p *packetCounter) received() {
	p.mu.Lock()
	p.in++
	p.mu.Unlock()
}

// report hands the packets counted so far to the connection.
func (p *packetCounter) report() {
	p.mu.Lock()
	in, out := p.in, p.out
	p.mu.Unlock()
	p.count.packets(in, out)
}
