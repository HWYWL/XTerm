package adb

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"strings"

	"golang.org/x/crypto/hkdf"

	"github.com/pgaskin/go-adb/adb/adbproto/aproto"
)

// The pairing protocol of Android 11 and later's wireless debugging: a
// TLS 1.3 session whose exported key, with the code the device shows, is
// the password of a SPAKE2 exchange; the key it derives seals the RSA
// public key this computer offers, which the device keeps if the exchange
// proved the code.

// The parts of the protocol that are written in bytes.
const (
	pairExportLabel = "adb-label\x00" // the TLS exporter's label
	pairExportSize  = 64
	pairClientName  = "adb pair client\x00"
	pairServerName  = "adb pair server\x00"
	pairAEADInfo    = "adb pairing_auth aes-128-gcm key"

	pairHeaderVersion = 1
	pairTypeSpake2    = 0
	pairTypePeerInfo  = 1
	pairInfoSize      = 8192
	pairInfoRSAKey    = 0
)

// ErrPairing says the exchange with the device failed: a code the device
// did not take, its pairing window closed, or a device that does not speak
// this protocol.
var ErrPairing = errors.New("the device refused the pairing")

// Pair pairs the device at addr, the port of its pairing window, with key:
// the code on the device's screen proves the exchange, and the device then
// trusts key's certificate, which is the one the app connects with.
func Pair(ctx context.Context, addr, code string, key crypto.Signer) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errors.New("adb: no pairing code")
	}
	if _, ok := key.Public().(*rsa.PublicKey); !ok {
		return errors.New("adb: the ADB key is not RSA")
	}
	cert, err := aproto.GenerateCertificate(key)
	if err != nil {
		return fmt.Errorf("adb: certificate: %w", err)
	}
	var d net.Dialer
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	tc := tls.Client(nc, &tls.Config{
		Certificates:       []tls.Certificate{{Certificate: [][]byte{cert}, PrivateKey: key}},
		InsecureSkipVerify: true, // the device's own certificate is not signed
		MinVersion:         tls.VersionTLS13,
	})
	defer tc.Close()
	if err := tc.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("adb: pairing handshake: %w", err)
	}
	state := tc.ConnectionState()
	exported, err := state.ExportKeyingMaterial(pairExportLabel, nil, pairExportSize)
	if err != nil {
		return fmt.Errorf("adb: pairing key: %w", err)
	}

	// The code, bound to this session by the exported key, is the SPAKE2
	// password; a code that is not the device's makes the sealed key
	// below unreadable to it.
	spake := newSpake2(true, []byte(pairClientName), []byte(pairServerName))
	msg, err := spake.generate(append([]byte(code), exported...))
	if err != nil {
		return err
	}
	if err := writePairPacket(tc, pairTypeSpake2, msg); err != nil {
		return pairingErr(err)
	}
	typ, their, err := readPairPacket(tc)
	if err != nil {
		return pairingErr(err)
	}
	if typ != pairTypeSpake2 || len(their) != 32 {
		return fmt.Errorf("%w: no SPAKE2 message", ErrPairing)
	}
	secret, err := spake.process(their)
	if err != nil {
		return pairingErr(err)
	}
	aead, err := pairAEAD(secret)
	if err != nil {
		return err
	}

	// The public key the device is to trust, sealed with the exchange's
	// key: the device reads it only if the code was right.
	info, err := pairPeerInfo(key)
	if err != nil {
		return err
	}
	if err := writePairPacket(tc, pairTypePeerInfo, aead.Seal(nil, pairNonce(0), info, nil)); err != nil {
		return pairingErr(err)
	}
	typ, sealed, err := readPairPacket(tc)
	if err != nil {
		return pairingErr(err)
	}
	if typ != pairTypePeerInfo {
		return fmt.Errorf("%w: no device information", ErrPairing)
	}
	if _, err := aead.Open(nil, pairNonce(0), sealed, nil); err != nil {
		return fmt.Errorf("%w: the code was not the device's", ErrPairing)
	}
	return nil
}

// pairingErr is a failure of the exchange itself, which a device that
// closed the connection for a wrong code causes as much as one that does
// not speak the protocol.
func pairingErr(err error) error {
	return fmt.Errorf("%w: %v", ErrPairing, err)
}

// pairPeerInfo is the information a pairing sends: the key's public half,
// in the encoding the device keeps in its list of trusted keys, in a fixed
// 8192-byte message.
func pairPeerInfo(key crypto.Signer) ([]byte, error) {
	pub, ok := key.Public().(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("adb: the ADB key is not RSA")
	}
	pkey, err := aproto.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("adb: public key: %w", err)
	}
	data := aproto.AppendPublicKey(nil, pkey, pairKeyName())
	info := make([]byte, pairInfoSize)
	info[0] = pairInfoRSAKey
	if len(data) >= pairInfoSize {
		return nil, errors.New("adb: the public key is too long")
	}
	copy(info[1:], data)
	return info, nil
}

// pairKeyName is the name the device shows the key by: adb's user@host.
func pairKeyName() string {
	name, host := "unknown", "unknown"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		host = h
	}
	return name + "@" + host
}

// pairAEAD is the cipher the exchange's key seals the messages with.
func pairAEAD(secret []byte) (cipher.AEAD, error) {
	r := hkdf.New(sha256.New, secret, nil, []byte(pairAEADInfo))
	key := make([]byte, 16)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// pairNonce is the counter the protocol numbers its messages with, in the
// low eight bytes of a twelve-byte nonce.
func pairNonce(n uint64) []byte {
	nonce := make([]byte, 12)
	binary.LittleEndian.PutUint64(nonce, n)
	return nonce
}

// writePairPacket writes one message: its version, its type, its length,
// and its payload.
func writePairPacket(w io.Writer, typ byte, payload []byte) error {
	header := make([]byte, 6)
	header[0] = pairHeaderVersion
	header[1] = typ
	binary.BigEndian.PutUint32(header[2:], uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readPairPacket reads one message of the protocol.
func readPairPacket(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 6)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	if header[0] != pairHeaderVersion {
		return 0, nil, fmt.Errorf("unknown version %d", header[0])
	}
	n := binary.BigEndian.Uint32(header[2:])
	if n > 2*pairInfoSize {
		return 0, nil, fmt.Errorf("a message of %d bytes", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return header[1], payload, nil
}
