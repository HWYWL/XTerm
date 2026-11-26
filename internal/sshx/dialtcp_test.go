package sshx

import (
	"io"
	"net"
	"testing"
	"time"

	"finallyshell/internal/config"
)

// echoOn sends msg on c and waits for it back.
func echoOn(t *testing.T, c net.Conn, msg string) {
	t.Helper()
	defer c.Close()
	if _, err := io.WriteString(c, msg); err != nil {
		t.Fatalf("%v", err)
	}
	buf := make([]byte, len(msg))
	if err := c.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatalf("%v", err)
	}
	if string(buf) != msg {
		t.Errorf("the echo is %q, want %q", buf, msg)
	}
}

// DialTCP reaches a server the way its host's proxy says: a protocol that
// is not SSH, as Redis, connects to each address of its own through it.
func TestDialTCPThroughProxy(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	if echo == nil {
		t.Skip("no loopback echo")
	}
	open := startSOCKS(t, "", "")
	h := &config.Host{ID: "cache", Proxy: &config.ProxyConfig{
		Kind: config.ProxySOCKS5, Addr: open.host, Port: open.port,
	}}
	conn, err := DialTCP(h, Options{}, echo.String(), 5*time.Second)
	if err != nil {
		t.Fatalf("SOCKS5: %v", err)
	}
	echoOn(t, conn, "hello")

	web := startHTTPProxy(t, "", "")
	h.Proxy = &config.ProxyConfig{Kind: config.ProxyHTTP, Addr: web.host, Port: web.port}
	conn, err = DialTCP(h, Options{}, startHello(t), 5*time.Second)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 2)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	if string(buf) != "hi" {
		t.Errorf("the server said %q, want %q", buf, "hi")
	}
}

// startHello starts a server that says hello as it accepts, and then
// echoes: an HTTP proxy answers once it has read from the server it opens.
func startHello(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.WriteString(c, "hi")
				io.Copy(c, c)
			}(c)
		}
	}()
	return ln.Addr().String()
}

// A proxy that asks for a password takes the one the options hold.
func TestDialTCPProxyPassword(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	if echo == nil {
		t.Skip("no loopback echo")
	}
	open := startSOCKS(t, "alice", "secret")
	h := &config.Host{ID: "cache", Proxy: &config.ProxyConfig{
		Kind: config.ProxySOCKS5, Addr: open.host, Port: open.port, User: "alice",
	}}
	if _, err := DialTCP(h, Options{}, echo.String(), 5*time.Second); err == nil {
		t.Error("a proxy without its password answered")
	}
	conn, err := DialTCP(h, Options{ProxyPassword: "secret"}, echo.String(), 5*time.Second)
	if err != nil {
		t.Fatalf("%v", err)
	}
	echoOn(t, conn, "hello")
}

// A host without a jump host has no jump host's connection to give.
func TestJumpClientNone(t *testing.T) {
	c, err := JumpClient(Options{})
	if c != nil || err != nil {
		t.Errorf("client %v, err %v", c, err)
	}
}
