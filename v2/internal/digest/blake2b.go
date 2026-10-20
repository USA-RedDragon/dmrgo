package digest

import "math/bits"

// BLAKE2b is a keyed BLAKE2b hash in progress, with a digest of 1 to 64
// octets. The zero value is not ready; call Init first.
type BLAKE2b struct {
	h      [8]uint64
	t      uint64
	buf    [128]byte
	n      uint8
	size   uint8
	key    [64]byte
	keyLen uint8
}

// Init sets the digest size and the key, which may be empty, and starts a
// hash. It reports false, and does nothing, when size is not 1 to 64 or the
// key is longer than 64 octets.
func (d *BLAKE2b) Init(size int, key []byte) bool {
	if size < 1 || size > 64 || len(key) > 64 {
		return false
	}
	clear(d.key[:])
	d.size, d.keyLen = uint8(size&0xFF), uint8(copy(d.key[:], key)&0xFF)
	d.Reset()
	return true
}

func blake2bIV() [8]uint64 {
	return [8]uint64{0x6A09E667F3BCC908, 0xBB67AE8584CAA73B, 0x3C6EF372FE94F82B, 0xA54FF53A5F1D36F1,
		0x510E527FADE682D1, 0x9B05688C2B3E6C1F, 0x1F83D9ABFB41BD6B, 0x5BE0CD19137E2179}
}

// Reset starts a new hash with the size and key of Init.
func (d *BLAKE2b) Reset() {
	d.h = blake2bIV()
	d.h[0] ^= 0x01010000 ^ uint64(d.keyLen)<<8 ^ uint64(d.size)
	d.t, d.n = 0, 0
	clear(d.buf[:])
	if d.keyLen > 0 {
		copy(d.buf[:], d.key[:d.keyLen])
		d.n = 128
	}
}

// Write adds p to the hash.
func (d *BLAKE2b) Write(p []byte) {
	for len(p) > 0 {
		if d.n == 128 {
			d.t += 128
			d.compress(false)
			d.n = 0
		}
		k := copy(d.buf[d.n:], p)
		d.n += uint8(k & 0xFF)
		p = p[k:]
	}
}

// Sum ends the hash and writes its first size octets to out, which must
// hold them. Call Reset before the next hash.
func (d *BLAKE2b) Sum(out []byte) {
	d.t += uint64(d.n)
	clear(d.buf[d.n:])
	d.compress(true)
	for i := range d.size {
		out[i] = byte(d.h[i/8] >> (8 * (i % 8)) & 0xFF)
	}
}

func (d *BLAKE2b) compress(last bool) {
	sigma := [12][16]uint8{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
		{11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4},
		{7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8},
		{9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13},
		{2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9},
		{12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11},
		{13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10},
		{6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5},
		{10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0},
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
	}
	var m [16]uint64
	for i := range 16 {
		for j := range 8 {
			m[i] |= uint64(d.buf[8*i+j]) << (8 * j)
		}
	}
	iv := blake2bIV()
	var v [16]uint64
	copy(v[:8], d.h[:])
	copy(v[8:], iv[:])
	v[12] ^= d.t
	if last {
		v[14] = ^v[14]
	}
	for r := range 12 {
		s := &sigma[r]
		for i := range 4 {
			a, b, c, e := i, i+4, i+8, i+12
			v[a], v[b], v[c], v[e] = mix(v[a], v[b], v[c], v[e], m[s[2*i]], m[s[2*i+1]])
		}
		for i := range 4 {
			a, b, c, e := i, 4+(i+1)%4, 8+(i+2)%4, 12+(i+3)%4
			v[a], v[b], v[c], v[e] = mix(v[a], v[b], v[c], v[e], m[s[8+2*i]], m[s[8+2*i+1]])
		}
	}
	for i := range 8 {
		d.h[i] ^= v[i] ^ v[i+8]
	}
}

func mix(a, b, c, d, x, y uint64) (uint64, uint64, uint64, uint64) {
	a += b + x
	d = bits.RotateLeft64(d^a, -32)
	c += d
	b = bits.RotateLeft64(b^c, -24)
	a += b + y
	d = bits.RotateLeft64(d^a, -16)
	c += d
	b = bits.RotateLeft64(b^c, -63)
	return a, b, c, d
}
