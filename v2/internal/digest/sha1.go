// Package digest holds SHA-1, SHA-256 and keyed BLAKE2b as concrete types
// that do not allocate and keep their state in the value, for the login
// token and the signatures of the transport packages. (FIPS 180-4; RFC
// 2104; RFC 7693)
package digest

import "math/bits"

// SHA1 is a SHA-1 hash in progress. The zero value is not ready; call
// Reset first.
type SHA1 struct {
	h   [5]uint32
	buf [64]byte
	n   int
	len uint64
}

// Reset starts a new hash.
func (d *SHA1) Reset() {
	d.h = [5]uint32{0x67452301, 0xEFCDAB89, 0x98BADCFE, 0x10325476, 0xC3D2E1F0}
	d.n, d.len = 0, 0
}

// Write adds p to the hash.
func (d *SHA1) Write(p []byte) {
	d.len += uint64(len(p))
	for len(p) > 0 {
		k := copy(d.buf[d.n:], p)
		d.n += k
		p = p[k:]
		if d.n == 64 {
			d.block()
			d.n = 0
		}
	}
}

// Sum returns the hash of what was written. It leaves d as it is.
func (d SHA1) Sum() [20]byte {
	e := d
	e.pad()
	var out [20]byte
	for i, v := range e.h {
		out[4*i], out[4*i+1], out[4*i+2], out[4*i+3] = byte(v>>24&0xFF), byte(v>>16&0xFF), byte(v>>8&0xFF), byte(v&0xFF)
	}
	return out
}

func (d *SHA1) pad() {
	n := d.len
	var tail [72]byte
	tail[0] = 0x80
	k := 56 - d.n
	if k <= 0 {
		k += 64
	}
	for i := range 8 {
		tail[k+i] = byte(n << 3 >> (56 - 8*i) & 0xFF)
	}
	d.Write(tail[:k+8])
}

func (d *SHA1) block() {
	var w [16]uint32
	for i := range 16 {
		w[i] = uint32(d.buf[4*i])<<24 | uint32(d.buf[4*i+1])<<16 | uint32(d.buf[4*i+2])<<8 | uint32(d.buf[4*i+3])
	}
	a, b, c, dd, e := d.h[0], d.h[1], d.h[2], d.h[3], d.h[4]
	for i := range 80 {
		if i >= 16 {
			w[i&15] = bits.RotateLeft32(w[(i-3)&15]^w[(i-8)&15]^w[(i-14)&15]^w[i&15], 1)
		}
		var f, k uint32
		switch {
		case i < 20:
			f, k = b&c|^b&dd, 0x5A827999
		case i < 40:
			f, k = b^c^dd, 0x6ED9EBA1
		case i < 60:
			f, k = b&c|b&dd|c&dd, 0x8F1BBCDC
		default:
			f, k = b^c^dd, 0xCA62C1D6
		}
		t := bits.RotateLeft32(a, 5) + f + e + k + w[i&15]
		a, b, c, dd, e = t, a, bits.RotateLeft32(b, 30), c, dd
	}
	d.h[0] += a
	d.h[1] += b
	d.h[2] += c
	d.h[3] += dd
	d.h[4] += e
}

// HMACSHA1 is an HMAC-SHA1 key with its hash state. The zero value is not
// ready; call Init first.
type HMACSHA1 struct {
	opad  [64]byte
	inner SHA1
	start SHA1
}

// Init sets the key. A key longer than 64 octets is hashed first, as RFC
// 2104 says.
func (m *HMACSHA1) Init(key []byte) {
	var k [64]byte
	if len(key) > 64 {
		m.start.Reset()
		m.start.Write(key)
		sum := m.start.Sum()
		copy(k[:], sum[:])
	} else {
		copy(k[:], key)
	}
	var ipad [64]byte
	for i := range 64 {
		ipad[i], m.opad[i] = k[i]^0x36, k[i]^0x5C
	}
	m.start.Reset()
	m.start.Write(ipad[:])
	m.inner = m.start
}

// Reset starts a new message.
func (m *HMACSHA1) Reset() { m.inner = m.start }

// Write adds p to the message.
func (m *HMACSHA1) Write(p []byte) { m.inner.Write(p) }

// Sum returns the HMAC of the message.
func (m *HMACSHA1) Sum() [20]byte {
	in := m.inner.Sum()
	var o SHA1
	o.Reset()
	o.Write(m.opad[:])
	o.Write(in[:])
	return o.Sum()
}
