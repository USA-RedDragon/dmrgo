// Package auth computes and checks the login token of package dmrd and the
// signatures of OpenBridge packets: SHA-256, HMAC-SHA1 and keyed BLAKE2b.
// Package dmrd has none of this code, so a program that only encodes and
// decodes does not link it. Nothing here allocates but NewBridge.
//
// (DMRGateway ee79d38 DMRNetwork.cpp writeAuthorisation; DL5DI, G4KLX and
// DG1HT, IPSC Protocol Specs for homebrew DMR repeater, 2015-07-26;
// BrandMeister Open Bridge; HBlink3 1db2730; FreeDMR 546165eb)
package auth

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

// Token returns the login token of an RPTK message for the salt of the
// master and the password: SHA-256 of the salt and the password. In
// DialectMMDVM the salt is its 4 octets as RPTACK sent them; in DialectHBRP
// it is its 8 hex digits in upper case, as in the example of the
// specification. It returns an error for another dialect, and does not
// allocate.
func Token(d dmrd.Dialect, salt uint32, password []byte) ([32]byte, error) {
	var prefix [8]byte
	var n int
	switch d {
	case dmrd.DialectMMDVM:
		prefix[0], prefix[1], prefix[2], prefix[3] = byte(salt>>24&0xFF), byte(salt>>16&0xFF), byte(salt>>8&0xFF), byte(salt&0xFF)
		n = 4
	case dmrd.DialectHBRP:
		for i := range 8 {
			prefix[i] = "0123456789ABCDEF"[salt>>(28-4*i)&0xF]
		}
		n = 8
	case dmrd.DialectUnknown, dmrd.DialectLocal:
		return [32]byte{}, errDialect
	default:
		return [32]byte{}, errDialect
	}
	var h digest.SHA256
	h.Reset()
	h.Write(prefix[:n])
	h.Write(password)
	return h.Sum(), nil
}

// VerifyToken reports whether token, from an RPTK message, is the login
// token of the salt and the password, as Token computes it. It compares
// in constant time, and reports false for a dialect without a token. It
// does not allocate.
func VerifyToken(d dmrd.Dialect, salt uint32, password, token []byte) bool {
	want, err := Token(d, salt, password)
	return err == nil && digest.Equal(want[:], token)
}

// Bridge holds an OpenBridge key and the hash state that signing with it
// needs. It is not safe for concurrent use.
type Bridge struct {
	hmac digest.HMACSHA1
	b2   digest.BLAKE2b
}

// KeyLen is the length of an OpenBridge key in octets.
const KeyLen = 20

// NewBridge returns a Bridge for key, the passphrase of the OpenBridge link
// as plain octets. As HBlink3 and FreeDMR do, it pads the passphrase with
// zero octets to KeyLen and cuts it at KeyLen. The padding does not change
// HMAC-SHA1, but the BLAKE2b hash of FreeDMR versions 4 and 5 is keyed
// with all KeyLen octets. It allocates the Bridge.
//
// (HBlink3 1db2730 and FreeDMR config.py, the PASSPHRASE of an OPENBRIDGE
// section)
func NewBridge(key []byte) *Bridge {
	var k [KeyLen]byte
	copy(k[:], key)
	b := &Bridge{}
	b.hmac.Init(k[:])
	b.b2.Init(16, k[:])
	return b
}

func (b *Bridge) sha1(msg []byte) [20]byte {
	b.hmac.Reset()
	b.hmac.Write(msg)
	return b.hmac.Sum()
}

func (b *Bridge) blake(msg []byte) [16]byte {
	var out [16]byte
	b.b2.Reset()
	b.b2.Write(msg)
	b.b2.Sum(out[:])
	return out
}

func span(p []byte) (msg, sig []byte, keyed bool, ok bool) {
	if len(p) < 4 {
		return nil, nil, false, false
	}
	switch {
	case tagIs(p, "DMRD") && len(p) == 73:
		return p[:53], p[53:], false, true
	case tagIs(p, "DMRE") && len(p) == 85:
		return p[:69], p[69:], true, true
	case tagIs(p, "DMRE") && len(p) == 89:
		return p[:73], p[73:], true, true
	case (tagIs(p, "BCKA") || tagIs(p, "BCST")) && len(p) == 24:
		return p[:4], p[4:], false, true
	case tagIs(p, "BCSQ") && len(p) == 31:
		return p[:11], p[11:], false, true
	case tagIs(p, "BCVE") && len(p) == 25:
		return p[4:5], p[5:], false, true
	}
	return nil, nil, false, false
}

func tagIs(p []byte, t string) bool {
	return p[0] == t[0] && p[1] == t[1] && p[2] == t[2] && p[3] == t[3]
}

// Sign writes the signature of the OpenBridge packet p, as
// dmrd.Bridge.AppendBinary and dmrd.BridgeControl.AppendBinary encode it,
// into its last octets: HMAC-SHA1 over the first 53 octets of a version 1
// packet and over the tag (and talkgroup and stream of BCSQ, or only the
// version octet of BCVE) of a control packet, and keyed BLAKE2b-128 over
// the octets before the hash of a DMRE packet. It returns an error, and
// changes nothing, when p is none of these. It does not allocate.
func (b *Bridge) Sign(p []byte) error {
	msg, sig, keyed, ok := span(p)
	if !ok {
		return errPacket
	}
	if keyed {
		h := b.blake(msg)
		copy(sig, h[:])
		return nil
	}
	h := b.sha1(msg)
	copy(sig, h[:])
	return nil
}

// Verify reports whether the OpenBridge packet p carries the signature of
// the key, as Sign would write it. It compares the signature in constant
// time, and does not allocate.
func (b *Bridge) Verify(p []byte) bool {
	msg, sig, keyed, ok := span(p)
	if !ok {
		return false
	}
	var want [20]byte
	n := len(want)
	if keyed {
		bl := b.blake(msg)
		copy(want[:], bl[:])
		n = len(bl)
	} else {
		want = b.sha1(msg)
	}
	return digest.Equal(want[:n], sig[:n])
}
