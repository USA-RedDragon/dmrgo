// Package auth signs and checks the authentication trailer of IPSC packets:
// the first 10 octets of HMAC-SHA1 of the packet, with a 20-octet key.
// Package ipsc has none of this code, so a program that only encodes and
// decodes does not link it. Nothing here allocates but NewKey.
//
// (DMRlink; ipsc2hbrp; captures of a Motorola XPR 8400, 2026-10-15)
package auth

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
)

// TrailerLen is the length of the authentication trailer in octets.
const TrailerLen = 10

var (
	errKeyText = errors.New("auth: key is not up to 40 hex digits")
)

// Key holds an IPSC authentication key and the HMAC state that signing
// with it needs. It is not safe for concurrent use.
type Key struct {
	mac digest.HMACSHA1
}

// ParseKey parses the hex key of a MOTOTRBO or DMRlink configuration: up
// to 40 hex digits, padded on the left with zeros to 20 octets as DMRlink
// does. It returns an error for more than 40 digits or a character that is
// not a hex digit, and does not allocate.
func ParseKey(s string) ([20]byte, error) {
	var k [20]byte
	if len(s) > 40 {
		return k, errKeyText
	}
	for i := range len(s) {
		c := s[len(s)-1-i]
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			return [20]byte{}, errKeyText
		}
		k[19-i/2] |= v << (4 * (i % 2))
	}
	return k, nil
}

// NewKey returns a Key for the 20-octet key k. It allocates the Key.
func NewKey(k [20]byte) *Key {
	key := &Key{}
	key.mac.Init(k[:])
	return key
}

func (k *Key) trailer(p []byte) [20]byte {
	k.mac.Reset()
	k.mac.Write(p)
	return k.mac.Sum()
}

// Sign appends the authentication trailer of the packet in dst to dst. It
// returns dst unchanged and dmrgo.ErrNilReceiver for a nil k. It allocates
// only when dst has no room for TrailerLen more octets.
func (k *Key) Sign(dst []byte) ([]byte, error) {
	if k == nil {
		return dst, dmrgo.ErrNilReceiver
	}
	t := k.trailer(dst)
	return append(dst, t[:TrailerLen]...), nil
}

// Verify reports whether the last TrailerLen octets of p are the trailer of
// the rest, and then returns the rest of p, the packet without its trailer.
// It reports false, with a nil body, otherwise and for a nil k. It compares the trailer in constant time and
// does not allocate.
func (k *Key) Verify(p []byte) (body []byte, ok bool) {
	if k == nil || len(p) < TrailerLen {
		return nil, false
	}
	body = p[:len(p)-TrailerLen]
	t := k.trailer(body)
	if !digest.Equal(t[:TrailerLen], p[len(body):]) {
		return nil, false
	}
	return body, true
}
