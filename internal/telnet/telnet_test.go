package telnet

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

// server starts a Telnet server on a port of this machine, and returns
// the client's end and the server's connection.
func server(t *testing.T) (*Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	select {
	case s := <-accepted:
		t.Cleanup(func() { s.Close() })
		return conn, s
	case <-time.After(5 * time.Second):
		t.Fatal("no connection")
		return nil, nil
	}
}

// readSome reads n bytes of what the server wrote.
func readSome(t *testing.T, c *Conn, n int) []byte {
	t.Helper()
	out := make([]byte, n)
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := range out {
		if _, err := c.Read(out[i : i+1]); err != nil {
			t.Fatalf("read: %v", err)
		}
	}
	return out
}

// The negotiations are answered, and the bytes around them are the
// terminal's: the stream of a server that asks for the terminal type, the
// window size and the echo, and then writes a prompt.
func TestNegotiation(t *testing.T) {
	c, s := server(t)
	// DO TTYPE, DO NAWS, WILL ECHO, and an option the client refuses.
	s.Write([]byte{iac, do, optTTYPE, iac, do, optNAWS, iac, will, optEcho})
	// The server asks for the terminal type by name.
	s.Write([]byte{iac, sb, optTTYPE, ttypeSend, iac, se})
	s.Write([]byte("login: "))

	got := string(readSome(t, c, 7))
	if got != "login: " {
		t.Errorf("read %q", got)
	}
	// The client answered the three options, and named its terminal.
	s.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 512)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	answers := buf[:n]
	for _, want := range [][]byte{
		{iac, will, optTTYPE},
		{iac, will, optNAWS},
		{iac, do, optEcho},
		{iac, sb, optTTYPE, ttypeIs, 'x'},
	} {
		if !bytes.Contains(answers, want) {
			t.Errorf("no answer %v in %v", want, answers)
		}
	}
}

// An option the client has no use for is refused, and the subnegotiation
// of one it does not know is dropped from the stream.
func TestRefusedOptionAndSubnegotiation(t *testing.T) {
	c, s := server(t)
	s.Write([]byte{iac, do, 42, iac, sb, 42, 1, 2, iac, se, 'o', 'k'})
	if got := string(readSome(t, c, 2)); got != "ok" {
		t.Errorf("read %q", got)
	}
	s.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 8)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	if want := []byte{iac, wont, 42}; !bytes.Equal(buf[:n], want) {
		t.Errorf("refused with %v, want %v", buf[:n], want)
	}
}

// 0xFF typed is sent doubled, and 0xFF sent is read once, as the protocol
// has it.
func TestDataEscape(t *testing.T) {
	c, s := server(t)
	if _, err := c.Write([]byte{0xFF, 'a'}); err != nil {
		t.Fatal(err)
	}
	s.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	if want := []byte{0xFF, 0xFF, 'a'}; !bytes.Equal(buf[:n], want) {
		t.Errorf("wrote %v, want %v", buf[:n], want)
	}
	s.Write([]byte{0xFF, 0xFF})
	if got := readSome(t, c, 1); !bytes.Equal(got, []byte{0xFF}) {
		t.Errorf("read %v", got)
	}
}

// Resizing sends the window's size, once the option is offered.
func TestResize(t *testing.T) {
	c, s := server(t)
	if err := c.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	if err := c.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	s.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	got := buf[:n]
	first := []byte{iac, will, optNAWS, iac, sb, optNAWS, 0, 120, 0, 40, iac, se}
	if !bytes.HasPrefix(got, first) {
		t.Errorf("resize wrote %v, want %v", got, first)
	}
	second := []byte{iac, sb, optNAWS, 0, 100, 0, 30, iac, se}
	if !bytes.Contains(got, second) {
		t.Errorf("second resize %v in %v", second, got)
	}
	if bytes.Count(got, []byte{iac, will, optNAWS}) != 1 {
		t.Errorf("the option offered %d times: %v", bytes.Count(got, []byte{iac, will, optNAWS}), got)
	}
}

// What the connection carried and the IAC commands it saw are counted for
// the System page: the bytes of the terminal's, the commands around them,
// the options the client answered, and the sequences a broken server
// sends.
func TestStats(t *testing.T) {
	c, s := server(t)
	// WILL ECHO, DO SGA, a NOP, a GA, a subnegotiation asking for the
	// terminal type, and a byte of data.
	s.Write([]byte{iac, will, optEcho, iac, do, optSGA, iac, nop, iac, ga})
	s.Write([]byte{iac, sb, optTTYPE, ttypeSend, iac, se})
	s.Write([]byte("hi"))
	if got := string(readSome(t, c, 2)); got != "hi" {
		t.Errorf("read %q", got)
	}
	if _, err := c.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := c.SendNOP(); err != nil {
		t.Fatal(err)
	}
	st := c.Stats()
	for _, c := range []struct {
		name string
		got  uint64
		want uint64
	}{
		{"RX", st.RX, 2},
		{"TX", st.TX, 3},
		{"NOP", st.NOP, 1},
		{"NOPsSent", st.NOPsSent, 1},
		{"GA", st.GA, 1},
		{"WILL", st.WILL, 1},
		{"DO", st.DO, 1},
		{"Subneg", st.Subneg, 1},
		{"Echo", st.Echo, 1},
		{"SGA", st.SGA, 1},
	} {
		if c.got != c.want {
			t.Errorf("%s counted %d, want %d", c.name, c.got, c.want)
		}
	}
	// The answers the client wrote are counted too, and the socket's
	// bytes are at least the terminal's.
	if st.SentDO == 0 || st.SentWILL == 0 {
		t.Errorf("sent negotiations %+v", st)
	}
	if st.WireRX < st.RX || st.WireTX < st.TX {
		t.Errorf("wire bytes %d/%d, terminal %d/%d", st.WireRX, st.WireTX, st.RX, st.TX)
	}
	// ECHO was offered by the server and SGA agreed by both sides; the
	// client echoes nothing itself.
	opts := c.Options()
	if opts.Echo != OptionOn {
		t.Errorf("echo %v", opts.Echo)
	}
	if opts.SGA != OptionOn {
		t.Errorf("sga %v", opts.SGA)
	}
	if got := c.Activity(); got.IsZero() {
		t.Error("no activity")
	}

	// A subnegotiation with no option, a command inside one, and an IAC
	// followed by no command at all are malformed, and counted rather
	// than shown on the terminal.
	s.Write([]byte{iac, sb, iac, se, iac, sb, optTTYPE, iac, nop, iac, se, iac, 5})
	s.Write([]byte("ok"))
	if got := string(readSome(t, c, 2)); got != "ok" {
		t.Errorf("after the malformed commands %q", got)
	}
	st = c.Stats()
	if st.Malformed < 2 {
		t.Errorf("malformed %d", st.Malformed)
	}
	if st.InvalidIAC != 1 {
		t.Errorf("invalid IAC %d", st.InvalidIAC)
	}
}

// A negotiation the client refuses, such as LINEMODE, is one the page can
// tell from one neither side asked for.
func TestRefusedOptionState(t *testing.T) {
	c, s := server(t)
	s.Write([]byte{iac, do, optLinemode})
	s.Write([]byte("x"))
	if got := string(readSome(t, c, 1)); got != "x" {
		t.Errorf("read %q", got)
	}
	if got := c.Options().Linemode; got != OptionRefused {
		t.Errorf("linemode %v", got)
	}
	if got := c.Options().Echo; got != OptionNone {
		t.Errorf("echo %v", got)
	}
}
