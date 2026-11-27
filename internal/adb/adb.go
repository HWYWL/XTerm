// Package adb is the client of an Android device over ADB: it opens a shell
// of the device as the connection a terminal reads and writes, over a cable
// through the ADB server of this computer, or straight to the device's own
// port on the network.
package adb

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	goadb "github.com/pgaskin/go-adb/adb"
	"github.com/pgaskin/go-adb/adb/adbhost"
	"github.com/pgaskin/go-adb/adb/adbproto"
	"github.com/pgaskin/go-adb/adb/adbproto/atransport"
	"github.com/pgaskin/go-adb/adb/adbproto/shellproto2"
	"github.com/pgaskin/go-adb/adb/adbtcpip"
	"github.com/pgaskin/go-adb/adb/adbusb"
	"github.com/pgaskin/go-adb/adblib"
	"github.com/pgaskin/go-adb/adblib/adbsync"
)

// Options is how a session reaches its device.
type Options struct {
	// Wireless reaches the device at Addr over the network, rather than
	// through the ADB server of this computer.
	Wireless bool
	// Addr is the device's address on the network, with its port; a
	// missing port is ADB's 5555.
	Addr string
	// Device is the serial of the device on a cable, as `adb devices`
	// shows it; empty takes the first one.
	Device string
	// Server is the ADB server of this computer; empty is its usual
	// address, localhost:5037.
	Server string
	// KeyPath is where the app keeps its own ADB key, made the first time
	// one is needed. The user's key, when adb made one, is used before
	// it, so that a device that trusts it asks nothing.
	KeyPath string
	// Term is the terminal type the device's shell is told; empty is
	// xterm-256color.
	Term string
}

// ErrNoServer says a device on a cable needs the ADB server of this
// computer, which is not running, on a system that cannot open the USB
// endpoint itself.
var ErrNoServer = errors.New("the ADB server of this computer is not running")

// ErrUnauthorized says the device did not allow this computer's key in
// time: its screen asks, on the first connection.
var ErrUnauthorized = errors.New("the device did not allow the ADB key")

// ErrNoDevice says no device is attached to the ADB server of this
// computer, or none of them has the serial that was asked for.
var ErrNoDevice = errors.New("no ADB device is attached")

// ErrDeviceNotReady says a device is attached, but cannot be used yet: it
// is offline, or its screen has not allowed this computer's key.
var ErrDeviceNotReady = errors.New("the ADB device is not ready")

// ErrRejected says the device dropped the connection as soon as it was
// made. Wireless debugging on Android 11 and later only lets in a computer
// it was paired with, and drops the rest without asking; over a cable it
// is a key the device does not trust.
var ErrRejected = errors.New("the device dropped the connection")

// Dial opens a shell of the device as the stream of a terminal, resizing
// its window as the terminal does. The device has to allow this computer's
// key first, which asks on its screen: ctx should leave it time to.
func Dial(ctx context.Context, o Options) (*Conn, error) {
	srv, _, closer, err := connect(ctx, o)
	if err != nil {
		return nil, err
	}
	count := &counters{}
	sh, err := openShell(ctx, srv, o.Term, count)
	if err != nil {
		if closer != nil {
			closer.Close()
		}
		return nil, err
	}
	c := &Conn{stream: sh, closer: closer, dial: srv, count: count,
		syncClient: &adbsync.Client{Server: srv, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 2}}
	c.count.connected()
	c.count.open(ChannelShell)
	return c, nil
}

// connect reaches the device: over the network at its own address, or over
// a cable through the ADB server of this computer, or, where the system
// lets the app open the USB endpoint itself, straight to it.
func connect(ctx context.Context, o Options) (goadb.Dialer, *atransport.Transport, io.Closer, error) {
	if o.Wireless {
		key, err := loadKey(o.KeyPath)
		if err != nil {
			return nil, nil, nil, err
		}
		t, err := (&adbtcpip.Dialer{
			Config: &atransport.Config{Keys: []crypto.Signer{key}},
		}).Connect(ctx, o.Addr)
		if err != nil {
			return nil, nil, nil, handshakeErr(ctx, err)
		}
		return t, t, t, nil
	}
	if serverRunning(ctx, o.Server) {
		// The running server owns the cable's devices: the shell is opened
		// through it, which finds the device by the serial, once the
		// device is there and ready.
		if err := checkDevices(ctx, o.Server, o.Device); err != nil {
			return nil, nil, nil, err
		}
		srv, err := adblib.Connect(ctx, o.Server, o.Device)
		if err != nil {
			return nil, nil, nil, err
		}
		return srv, nil, nil, nil
	}
	if !adbusb.Supported() {
		return nil, nil, nil, fmt.Errorf("%w: start `adb start-server`, or connect the device over the network", ErrNoServer)
	}
	key, err := loadKey(o.KeyPath)
	if err != nil {
		return nil, nil, nil, err
	}
	dev := adbusb.Any
	if o.Device != "" {
		dev = adbusb.Serial(o.Device)
	}
	t, err := (&adbusb.Dialer{
		Config: &atransport.Config{Keys: []crypto.Signer{key}},
	}).Connect(ctx, dev)
	if err != nil {
		return nil, nil, nil, handshakeErr(ctx, err)
	}
	return t, t, t, nil
}

// checkDevices says why the device on a cable cannot be used, from the
// list the ADB server keeps: none is attached, the one asked for is not
// there, or it is offline or has not allowed this computer.
func checkDevices(ctx context.Context, addr, serial string) error {
	d := &adbhost.Dialer{Addr: addr}
	devs, err := adbhost.Devices(ctx, d, true)
	if err != nil {
		return err
	}
	var found *adbhost.TransportInfo
	for _, t := range devs {
		if serial != "" && t.Serial != serial {
			continue
		}
		if found == nil || found.State != adbhost.CsDevice {
			found = t
		}
		if t.State == adbhost.CsDevice {
			break
		}
	}
	if found == nil {
		if serial != "" {
			return fmt.Errorf("%w: %q", ErrNoDevice, serial)
		}
		return ErrNoDevice
	}
	switch found.State {
	case adbhost.CsDevice:
		return nil
	case adbhost.CsUnauthorized, adbhost.CsAuthorizing:
		return ErrUnauthorized
	}
	return fmt.Errorf("%w: %s", ErrDeviceNotReady, found.State)
}

// handshakeErr is why the connection did not come up: a device that was
// asked to allow the key and did not, one that dropped the connection as
// it was made, or what the transport said.
func handshakeErr(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUnauthorized
	}
	// The transport reports a connection the device dropped as a kick
	// with no reason of its own.
	if strings.Contains(strings.ToLower(err.Error()), "kicked") {
		return fmt.Errorf("%w: %v", ErrRejected, err)
	}
	return err
}

// serverRunning tells whether an ADB server answers at addr, which the
// default is when it is empty.
func serverRunning(ctx context.Context, addr string) bool {
	if addr == "" {
		addr = adbhost.DefaultAddr
	}
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// LoadKey is the key a device is asked to allow, and the one a pairing
// offers: the user's own when adb made one, else the app's at path, made
// on its first use.
func LoadKey(path string) (crypto.Signer, error) { return loadKey(path) }

// loadKey is the key a device is asked to allow: the user's own when adb
// made one, so that a device already trusting it asks nothing, else the
// app's, made on its first use.
func loadKey(path string) (crypto.Signer, error) {
	if key, err := adbhost.LoadUserKey(); err == nil {
		return key, nil
	}
	if path == "" {
		return nil, errors.New("no ADB key to use")
	}
	if data, err := os.ReadFile(path); err == nil {
		return adbhost.ParseKey(data)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, block, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// openShell is an interactive shell of the device, with a pty where the
// device has the shell v2 protocol, and its plain shell service where it
// does not.
func openShell(ctx context.Context, srv goadb.Dialer, term string, count *counters) (io.ReadWriteCloser, error) {
	if term == "" {
		term = "xterm-256color"
	}
	if goadb.SupportsFeature(srv, adbproto.FeatureShell2) == nil {
		var b shellproto2.ServiceBuilder
		b.PTY()
		b.Term(term)
		sh, err := goadb.Shell2(ctx, srv, b.String())
		if err == nil {
			return newShell2(sh, count), nil
		}
		if ctx.Err() != nil {
			return nil, handshakeErr(ctx, err)
		}
		// The shell v2 service is not there after all: the plain shell
		// still is, without the window size.
	}
	sh, err := goadb.Shell(ctx, srv, "")
	if err != nil {
		return nil, handshakeErr(ctx, err)
	}
	return sh, nil
}

// shell2 is a shell v2 connection as a stream: the device's output is read
// out of its packets, what is typed goes back as its stdin, and its pty
// hears the terminal's size.
type shell2 struct {
	c     *goadb.ShellConn2
	count *counters
	mu    sync.Mutex // the writes, and the window size with them
	buf   []byte
	exit  bool
	err   error
}

func newShell2(c *goadb.ShellConn2, count *counters) *shell2 {
	return &shell2{c: c, count: count}
}

func (s *shell2) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(s.buf) == 0 {
		if s.err != nil {
			return 0, s.err
		}
		if s.exit {
			return 0, io.EOF
		}
		id, data, ok := s.c.Read()
		if !ok {
			s.err = s.c.Error()
			if s.err == nil {
				s.err = io.EOF
			}
			return 0, s.err
		}
		switch id {
		case shellproto2.PacketStdout, shellproto2.PacketStderr:
			if s.count != nil {
				s.count.packets(1, 0)
			}
			s.buf = append(s.buf[:0], data...)
		case shellproto2.PacketInvalid:
			if s.count != nil {
				s.count.malformed()
			}
		case shellproto2.PacketExit:
			// The shell ended: what it wrote before came first, and the
			// rest of the buffer is the end of it.
			s.exit = true
		}
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

func (s *shell2) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.c.Write(shellproto2.PacketStdin, p) {
		return 0, errOf(s.c)
	}
	if s.count != nil {
		s.count.packets(0, 1)
	}
	return len(p), nil
}

// Resize tells the device's pty the size of the terminal's window.
func (s *shell2) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	size := shellproto2.WinSize{Row: rows, Col: cols}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.c.Write(shellproto2.PacketWindowSizeChange, size.AppendBinary(nil)) {
		return errOf(s.c)
	}
	return nil
}

func (s *shell2) Close() error { return s.c.Close() }

// errOf is the connection's error, or a closed pipe where it has none.
func errOf(c *goadb.ShellConn2) error {
	if err := c.Error(); err != nil {
		return err
	}
	return io.ErrClosedPipe
}
