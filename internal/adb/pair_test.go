package adb

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// The two sides of the exchange agree when they share the password, and do
// not when they do not, or were told of each other by another name, as
// BoringSSL's own test of its SPAKE2 has it.
func TestSpake2Agrees(t *testing.T) {
	exchange := func(pa, pb []byte, an, bn [2][]byte) ([]byte, []byte, error) {
		a := newSpake2(true, an[0], an[1])
		b := newSpake2(false, bn[0], bn[1])
		am, err := a.generate(pa)
		if err != nil {
			return nil, nil, err
		}
		bm, err := b.generate(pb)
		if err != nil {
			return nil, nil, err
		}
		ak, err := a.process(bm)
		if err != nil {
			return nil, nil, err
		}
		bk, err := b.process(am)
		if err != nil {
			return nil, nil, err
		}
		return ak, bk, nil
	}
	names := [2][]byte{[]byte("adb pair client\x00"), []byte("adb pair server\x00")}
	peerNames := [2][]byte{[]byte("adb pair server\x00"), []byte("adb pair client\x00")}
	for i := 0; i < 3; i++ {
		ak, bk, err := exchange([]byte("123456"), []byte("123456"), names, peerNames)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ak, bk) {
			t.Fatal("the two sides of the same password disagree")
		}
		if len(ak) != 64 {
			t.Errorf("the key is %d bytes", len(ak))
		}
	}
	ak, bk, err := exchange([]byte("123456"), []byte("654321"), names, peerNames)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ak, bk) {
		t.Error("two passwords made the same key")
	}
	other := [2][]byte{[]byte("adb pair client"), []byte("adb pair server")}
	ak, bk, err = exchange([]byte("123456"), []byte("123456"), names, other)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ak, bk) {
		t.Error("the names are not in the transcript")
	}
}

// pairingDevice is a device's side of the pairing, for a test: it speaks
// the protocol as adbd does, with its own key.
type pairingDevice struct {
	code  string
	info  []byte // the information it answers with
	cert  tls.Certificate
	done  chan error
	conns chan struct{}
}

// startPairingDevice listens for a pairing, and answers with code's
// password. It returns the address to pair at.
func startPairingDevice(t *testing.T, code string) (*pairingDevice, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "adb pairing test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	d := &pairingDevice{
		code: code,
		info: func() []byte {
			info := make([]byte, pairInfoSize)
			info[0] = 1 // ADB_DEVICE_GUID
			copy(info[1:], "adb-1234")
			return info
		}(),
		cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		done: make(chan error, 4),
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{d.cert},
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tls.VersionTLS13,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go d.serve(t, c)
		}
	}()
	return d, ln.Addr().String()
}

// serve is one pairing, as the device does it.
func (d *pairingDevice) serve(t *testing.T, c net.Conn) {
	defer c.Close()
	tc, ok := c.(*tls.Conn)
	if !ok {
		d.done <- fmt.Errorf("not a TLS connection")
		return
	}
	if err := tc.HandshakeContext(context.Background()); err != nil {
		d.done <- err
		return
	}
	state := tc.ConnectionState()
	exported, err := state.ExportKeyingMaterial(pairExportLabel, nil, pairExportSize)
	if err != nil {
		d.done <- err
		return
	}
	spake := newSpake2(false, []byte(pairServerName), []byte(pairClientName))
	msg, err := spake.generate(append([]byte(d.code), exported...))
	if err != nil {
		d.done <- err
		return
	}
	typ, theirs, err := readPairPacket(tc)
	if err != nil || typ != pairTypeSpake2 {
		d.done <- fmt.Errorf("the client's SPAKE2 message: %v", err)
		return
	}
	secret, err := spake.process(theirs)
	if err != nil {
		d.done <- err
		return
	}
	if err := writePairPacket(tc, pairTypeSpake2, msg); err != nil {
		d.done <- err
		return
	}
	aead, err := pairAEAD(secret)
	if err != nil {
		d.done <- err
		return
	}
	typ, sealed, err := readPairPacket(tc)
	if err != nil || typ != pairTypePeerInfo {
		d.done <- fmt.Errorf("the client's information: %v", err)
		return
	}
	plain, err := aead.Open(nil, pairNonce(0), sealed, nil)
	if err != nil {
		d.done <- fmt.Errorf("the client's key does not open: %w", err)
		return
	}
	if len(plain) != pairInfoSize || plain[0] != pairInfoRSAKey {
		d.done <- fmt.Errorf("the client's information is %d bytes, type %d", len(plain), plain[0])
		return
	}
	if !bytes.HasPrefix(plain[1:], []byte("QAAA")) {
		d.done <- fmt.Errorf("the client's key starts %q", plain[1:12])
		return
	}
	if err := writePairPacket(tc, pairTypePeerInfo, aead.Seal(nil, pairNonce(0), d.info, nil)); err != nil {
		d.done <- err
		return
	}
	d.done <- nil
}

// A pairing the device's code agrees with succeeds, and leaves the device
// holding this computer's public key.
func TestPair(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	dev, addr := startPairingDevice(t, "123456")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Pair(ctx, addr, "123456", key); err != nil {
		t.Fatalf("pairing: %v", err)
	}
	select {
	case err := <-dev.done:
		if err != nil {
			t.Fatalf("the device: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the device heard nothing")
	}
}

// A code the device does not have is a pairing the device refuses, not one
// the app believes it made.
func TestPairWrongCode(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	dev, addr := startPairingDevice(t, "123456")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = Pair(ctx, addr, "000000", key)
	if !errors.Is(err, ErrPairing) {
		t.Fatalf("pairing with a wrong code: %v", err)
	}
	select {
	case err := <-dev.done:
		if err == nil {
			t.Fatal("the device took a wrong code")
		}
		if !strings.Contains(err.Error(), "does not open") {
			t.Errorf("the device's reason: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the device heard nothing")
	}
}

// A device that is not there, and a pairing with no code, are said as they
// are.
func TestPairNoDevice(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := Pair(ctx, addr, "123456", key); err == nil {
		t.Error("a pairing with no device succeeded")
	}
	if err := Pair(ctx, addr, "   ", key); err == nil {
		t.Error("a pairing with no code succeeded")
	}
}
