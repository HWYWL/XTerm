package adb

// This file is a port of BoringSSL's crypto/curve25519/spake25519.c, which
// Android's adbd and adb use for the wireless debugging pairing:
//
// Copyright (c) 2016, Google Inc.
//
// Permission to use, copy, modify, and/or distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY
// SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION
// OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN
// CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"math/big"

	"filippo.io/edwards25519"
)

// The two points SPAKE2 masks its messages with, as BoringSSL generates
// them: the hash of a fixed seed, decoded as an Edwards25519 point. They
// must be these points and not another suite's, or a device refuses the
// exchange.
var (
	spakeM = mustPoint("5ada7e4bf6ddd9adb6626d32131c6b5c51a1e347a3478f53cfcf441b88eed12e")
	spakeN = mustPoint("10e3df0ae37d8e7a99b5fe74b44672103dbddcbd06af680d71329a11693bc778")

	// spakeOrder is the order of the prime-order subgroup of
	// Edwards25519.
	spakeOrder, _ = new(big.Int).SetString("1000000000000000000000000000000014def9dea2f79cd65812631a5cf5d3ed", 16)
)

// mustPoint decodes a point, and panics on one that is not there: these
// are constants of the protocol.
func mustPoint(s string) *edwards25519.Point {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("adb: spake2 point: " + err.Error())
	}
	p, err := new(edwards25519.Point).SetBytes(b)
	if err != nil {
		panic("adb: spake2 point: " + err.Error())
	}
	return p
}

// spake2 is one side of a SPAKE2 exchange, as adb's pairing has it: the
// client is BoringSSL's Alice.
//
// The scalars are the integers BoringSSL works with, which run past the
// group's order: the points of the protocol are not all of the
// prime-order subgroup, and a scalar reduced before multiplying would
// leave another small-order part behind, and the device would derive
// another key.
type spake2 struct {
	client    bool
	myName    []byte
	theirName []byte

	myMsg        []byte
	passwordHash []byte
	password     *big.Int
	private      *big.Int
	generated    bool
}

func newSpake2(client bool, myName, theirName []byte) *spake2 {
	return &spake2{client: client, myName: myName, theirName: theirName}
}

// generate makes this side's message from the password, a 32-byte point
// the peer unmasks with its own.
func (s *spake2) generate(password []byte) ([]byte, error) {
	if s.generated {
		return nil, errors.New("adb: spake2 message already made")
	}
	seed := make([]byte, 64)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	// The same reduction of the random bytes as x25519_sc_reduce, and then
	// three left shifts: a multiple of the cofactor, so that the peer's
	// point, whatever of the small subgroup it holds, is cleared when the
	// shared secret is computed.
	ephemeral, err := new(edwards25519.Scalar).SetUniformBytes(seed)
	if err != nil {
		return nil, err
	}
	s.private = littleInt(ephemeral.Bytes())
	s.private.Lsh(s.private, 3)

	// The password's scalar, with the three low bits cleared as BoringSSL
	// does, so that the mask's small-order part says nothing of the
	// password.
	sum := sha512.Sum512(password)
	s.passwordHash = append([]byte(nil), sum[:]...)
	scalar, err := new(edwards25519.Scalar).SetUniformBytes(sum[:])
	if err != nil {
		return nil, err
	}
	s.password = littleInt(scalar.Bytes())
	for i := uint(0); i < 3; i++ {
		if s.password.Bit(int(i)) == 1 {
			s.password.Add(s.password, new(big.Int).Lsh(spakeOrder, i))
		}
	}

	// P* = x*B + w*M, the message the peer unmasks with the same
	// password.
	base := mulInt(edwards25519.NewGeneratorPoint(), s.private)
	mask := mulInt(s.maskPoint(), s.password)
	s.myMsg = new(edwards25519.Point).Add(base, mask).Bytes()
	s.generated = true
	return s.myMsg, nil
}

// process takes the peer's message, and returns the shared key, 64 bytes.
func (s *spake2) process(theirMsg []byte) ([]byte, error) {
	if !s.generated {
		return nil, errors.New("adb: no spake2 message made")
	}
	if len(theirMsg) != 32 {
		return nil, errors.New("adb: the peer's spake2 message is not a point")
	}
	peer, err := new(edwards25519.Point).SetBytes(theirMsg)
	if err != nil {
		return nil, errors.New("adb: the peer's spake2 message is not a point")
	}
	peersMask := mulInt(s.peerMaskPoint(), s.password)
	unmasked := new(edwards25519.Point).Subtract(peer, peersMask)
	shared := mulInt(unmasked, s.private)

	// The transcript, the client's words first on both sides, so that both
	// hash the same bytes.
	h := sha512.New()
	names := [2][]byte{s.myName, s.theirName}
	msgs := [2][]byte{s.myMsg, theirMsg}
	if !s.client {
		names = [2][]byte{s.theirName, s.myName}
		msgs = [2][]byte{theirMsg, s.myMsg}
	}
	writeLengthPrefixed(h, names[0])
	writeLengthPrefixed(h, names[1])
	writeLengthPrefixed(h, msgs[0])
	writeLengthPrefixed(h, msgs[1])
	writeLengthPrefixed(h, shared.Bytes())
	writeLengthPrefixed(h, s.passwordHash)
	return h.Sum(nil), nil
}

// maskPoint is the point this side's message is masked with, and
// peerMaskPoint the one the peer's is.
func (s *spake2) maskPoint() *edwards25519.Point {
	if s.client {
		return spakeM
	}
	return spakeN
}

func (s *spake2) peerMaskPoint() *edwards25519.Point {
	if s.client {
		return spakeN
	}
	return spakeM
}

// mulInt is p multiplied by k, the integer's own value and not k reduced
// modulo the group's order: the points of the protocol are not all of the
// prime-order subgroup, and BoringSSL multiplies by the integer. A fixed
// number of rounds keeps the time off the scalar's bits.
func mulInt(p *edwards25519.Point, k *big.Int) *edwards25519.Point {
	var bits [32]byte
	copy(bits[:], littleBytes(k))
	acc := edwards25519.NewIdentityPoint()
	v := new(edwards25519.Point).Set(p)
	next := new(edwards25519.Point)
	for i := 0; i < 256; i++ {
		add := next.Add(acc, v)
		acc = new(edwards25519.Point).Select(add, acc, int(bits[i/8]>>(i%8))&1)
		v.Double(v)
	}
	return acc
}

// littleInt is b, little-endian, as an integer.
func littleInt(b []byte) *big.Int { return new(big.Int).SetBytes(reverse(b)) }

// littleBytes is k, little-endian, at most 32 bytes.
func littleBytes(k *big.Int) []byte {
	b := reverse(k.Bytes())
	if len(b) > 32 {
		b = b[len(b)-32:]
	}
	return b
}

// writeLengthPrefixed writes data as the protocol hashes a field: its
// length, eight bytes little-endian, and then its bytes.
func writeLengthPrefixed(h hash.Hash, data []byte) {
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(data)))
	h.Write(n[:])
	h.Write(data)
}

// reverse is b the other way round, little-endian for big-endian.
func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[len(b)-1-i] = b[i]
	}
	return out
}
