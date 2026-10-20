package digest

import "math/bits"

// SHA256 is a SHA-256 hash in progress. The zero value is not ready; call
// Reset first.
type SHA256 struct {
	h   [8]uint32
	buf [64]byte
	n   int
	len uint64
}

// Reset starts a new hash.
func (d *SHA256) Reset() {
	d.h = [8]uint32{0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A, 0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19}
	d.n, d.len = 0, 0
}

// Write adds p to the hash.
func (d *SHA256) Write(p []byte) {
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
func (d SHA256) Sum() [32]byte {
	e := d
	n := e.len
	var tail [72]byte
	tail[0] = 0x80
	k := 56 - e.n
	if k <= 0 {
		k += 64
	}
	for i := range 8 {
		tail[k+i] = byte(n << 3 >> (56 - 8*i) & 0xFF)
	}
	e.Write(tail[:k+8])
	var out [32]byte
	for i, v := range e.h {
		out[4*i], out[4*i+1], out[4*i+2], out[4*i+3] = byte(v>>24&0xFF), byte(v>>16&0xFF), byte(v>>8&0xFF), byte(v&0xFF)
	}
	return out
}

func (d *SHA256) block() {
	k := [64]uint32{
		0x428A2F98, 0x71374491, 0xB5C0FBCF, 0xE9B5DBA5, 0x3956C25B, 0x59F111F1, 0x923F82A4, 0xAB1C5ED5,
		0xD807AA98, 0x12835B01, 0x243185BE, 0x550C7DC3, 0x72BE5D74, 0x80DEB1FE, 0x9BDC06A7, 0xC19BF174,
		0xE49B69C1, 0xEFBE4786, 0x0FC19DC6, 0x240CA1CC, 0x2DE92C6F, 0x4A7484AA, 0x5CB0A9DC, 0x76F988DA,
		0x983E5152, 0xA831C66D, 0xB00327C8, 0xBF597FC7, 0xC6E00BF3, 0xD5A79147, 0x06CA6351, 0x14292967,
		0x27B70A85, 0x2E1B2138, 0x4D2C6DFC, 0x53380D13, 0x650A7354, 0x766A0ABB, 0x81C2C92E, 0x92722C85,
		0xA2BFE8A1, 0xA81A664B, 0xC24B8B70, 0xC76C51A3, 0xD192E819, 0xD6990624, 0xF40E3585, 0x106AA070,
		0x19A4C116, 0x1E376C08, 0x2748774C, 0x34B0BCB5, 0x391C0CB3, 0x4ED8AA4A, 0x5B9CCA4F, 0x682E6FF3,
		0x748F82EE, 0x78A5636F, 0x84C87814, 0x8CC70208, 0x90BEFFFA, 0xA4506CEB, 0xBEF9A3F7, 0xC67178F2,
	}
	var w [16]uint32
	for i := range 16 {
		w[i] = uint32(d.buf[4*i])<<24 | uint32(d.buf[4*i+1])<<16 | uint32(d.buf[4*i+2])<<8 | uint32(d.buf[4*i+3])
	}
	a, b, c, dd, e, f, g, h := d.h[0], d.h[1], d.h[2], d.h[3], d.h[4], d.h[5], d.h[6], d.h[7]
	for i := range 64 {
		if i >= 16 {
			w15, w2 := w[(i-15)&15], w[(i-2)&15]
			s0 := bits.RotateLeft32(w15, -7) ^ bits.RotateLeft32(w15, -18) ^ w15>>3
			s1 := bits.RotateLeft32(w2, -17) ^ bits.RotateLeft32(w2, -19) ^ w2>>10
			w[i&15] += s0 + w[(i-7)&15] + s1
		}
		s1 := bits.RotateLeft32(e, -6) ^ bits.RotateLeft32(e, -11) ^ bits.RotateLeft32(e, -25)
		t1 := h + s1 + (e&f ^ ^e&g) + k[i] + w[i&15]
		s0 := bits.RotateLeft32(a, -2) ^ bits.RotateLeft32(a, -13) ^ bits.RotateLeft32(a, -22)
		t2 := s0 + (a&b ^ a&c ^ b&c)
		h, g, f, e, dd, c, b, a = g, f, e, dd+t1, c, b, a, t1+t2
	}
	d.h[0] += a
	d.h[1] += b
	d.h[2] += c
	d.h[3] += dd
	d.h[4] += e
	d.h[5] += f
	d.h[6] += g
	d.h[7] += h
}
