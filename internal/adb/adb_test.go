package adb

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	goadb "github.com/pgaskin/go-adb/adb"
	"github.com/pgaskin/go-adb/adb/adbproto/shellproto2"
)

// packet is a shell v2 packet the test's device sent or read.
type packet struct {
	id   shellproto2.PacketID
	data string
}

// A shell v2 connection is a stream for the terminal: the device's output
// comes out of its packets, the keys go back as its stdin, and the
// terminal's size reaches the device's pty.
func TestShell2Streams(t *testing.T) {
	client, device := net.Pipe()
	t.Cleanup(func() { device.Close() })
	sh := newShell2(goadb.NewShellConn2(client), &counters{})
	srv := shellproto2.New(device)
	got := make(chan packet, 2)
	go func() {
		srv.Write(shellproto2.PacketStdout, []byte("hel"))
		srv.Write(shellproto2.PacketStdout, []byte("lo"))
		srv.Write(shellproto2.PacketExit, nil)
		for range 2 {
			id, data, ok := srv.Read()
			if !ok {
				return
			}
			got <- packet{id, string(data)}
		}
	}()
	text, err := io.ReadAll(sh)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(text) != "hello" {
		t.Errorf("the stream %q", text)
	}
	if _, err := sh.Write([]byte("ls\r")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := sh.Resize(80, 24); err != nil {
		t.Fatalf("resize: %v", err)
	}
	select {
	case p := <-got:
		if p.id != shellproto2.PacketStdin || p.data != "ls\r" {
			t.Errorf("the device read %v %q", p.id, p.data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the device read nothing")
	}
	select {
	case p := <-got:
		if p.id != shellproto2.PacketWindowSizeChange || p.data != "24x80,0x0" {
			t.Errorf("the device heard the size %v %q", p.id, p.data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the device heard no size")
	}
}

// A shell the device ended is the stream's end, after what it wrote.
func TestShell2Ends(t *testing.T) {
	client, device := net.Pipe()
	t.Cleanup(func() { device.Close() })
	sh := newShell2(goadb.NewShellConn2(client), &counters{})
	go func() {
		srv := shellproto2.New(device)
		srv.Write(shellproto2.PacketStdout, []byte("bye"))
		srv.Write(shellproto2.PacketExit, nil)
	}()
	buf := make([]byte, 3)
	if _, err := io.ReadFull(sh, buf); err != nil || string(buf) != "bye" {
		t.Fatalf("read %q, %v", buf, err)
	}
	if _, err := sh.Read(buf); err != io.EOF {
		t.Errorf("after the exit: %v", err)
	}
}

// The app's key is made the first time and read back after, without the
// device being asked to trust a new one on every run.
func TestLoadKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows reads this one
	path := filepath.Join(t.TempDir(), "adbkey")
	first, err := loadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	key, ok := first.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("the key is %T", first)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the key was not written: %v", err)
	}
	if blk, _ := pem.Decode(data); blk == nil || !bytes.Contains(data, []byte("PRIVATE KEY")) {
		t.Errorf("the key file %q", data)
	}
	second, err := loadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	again, ok := second.(*rsa.PrivateKey)
	if !ok || again.N.Cmp(key.N) != 0 || again.E != key.E {
		t.Error("the key changed between runs")
	}
}

// A server that answers is found, and one that is not there is not.
func TestServerRunning(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	ctx := context.Background()
	if !serverRunning(ctx, ln.Addr().String()) {
		t.Error("the server was not found")
	}
	ln.Close()
	if serverRunning(ctx, ln.Addr().String()) {
		t.Error("a closed port was found")
	}
}

// A device that drops the connection as it is made is a rejected one, a
// handshake that ran out of time is a key the device did not allow, and
// anything else is kept as it came.
func TestHandshakeErr(t *testing.T) {
	// A connection the user stopped is not a device that said no.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := handshakeErr(ctx, context.Canceled); errors.Is(err, ErrUnauthorized) {
		t.Errorf("a cancelled handshake: %v", err)
	}
	dead, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	if err := handshakeErr(dead, context.DeadlineExceeded); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("a timeout: %v", err)
	}
	kicked := errors.New("kicked: kicked")
	if err := handshakeErr(context.Background(), kicked); !errors.Is(err, ErrRejected) {
		t.Errorf("a kicked connection: %v", err)
	}
	other := errors.New("connection reset by peer")
	if err := handshakeErr(context.Background(), other); !errors.Is(err, other) || errors.Is(err, ErrRejected) {
		t.Errorf("another failure: %v", err)
	}
}
