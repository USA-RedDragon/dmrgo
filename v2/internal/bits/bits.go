// Package bits reads and writes bit fields of packed byte slices, the first
// bit in the most significant bit of the first byte, the order of a DMR
// burst on air. It converts between packed bytes and bit.Bit slices at the
// edges of the public API. A function panics when a slice is shorter than the
// bits it reads or writes.
package bits

import "github.com/USA-RedDragon/dmrgo/v2/bit"

// Bit returns bit i of b, 0 or 1.
func Bit(b []byte, i int) byte {
	return b[i>>3] >> (7 - i&7) & 1
}

// SetBit sets bit i of b to the low bit of v.
func SetBit(b []byte, i int, v byte) {
	m := byte(0x80) >> (i & 7)
	if v&1 != 0 {
		b[i>>3] |= m
	} else {
		b[i>>3] &^= m
	}
}

// Field returns the n bits of b from bit start as an unsigned integer, the
// first bit most significant. n is at most 57, so that the field fits in the
// 8 bytes that Field reads.
func Field(b []byte, start, n int) uint64 {
	if n == 0 {
		return 0
	}
	first, last := start>>3, (start+n-1)>>3
	var v uint64
	for _, x := range b[first : last+1] {
		v = v<<8 | uint64(x)
	}
	return v >> (7 - (start+n-1)&7) & (1<<n - 1)
}

// SetField writes the low n bits of v into b from bit start, the most
// significant of them first, and leaves the other bits of b as they are. n
// is at most 57, as for Field.
func SetField(b []byte, start, n int, v uint64) {
	if n == 0 {
		return
	}
	first, last := start>>3, (start+n-1)>>3
	shift := 7 - (start+n-1)&7
	mask := (uint64(1)<<n - 1) << shift
	v = v << shift & mask
	for i := last; i >= first; i-- {
		b[i] = b[i]&^byte(mask) | byte(v)
		mask >>= 8
		v >>= 8
	}
}

// Field64 returns the n bits of b from bit start, as Field does, for n up
// to 64.
func Field64(b []byte, start, n int) uint64 {
	if n <= 32 {
		return Field(b, start, n)
	}
	return Field(b, start, n-32)<<32 | Field(b, start+n-32, 32)
}

// SetField64 writes the low n bits of v into b from bit start, as SetField
// does, for n up to 64.
func SetField64(b []byte, start, n int, v uint64) {
	if n <= 32 {
		SetField(b, start, n, v)
		return
	}
	SetField(b, start, n-32, v>>32)
	SetField(b, start+n-32, 32, v)
}

// Copy copies n bits of src from bit srcStart to dst from bit dstStart. The
// other bits of dst stay as they are. src and dst must not overlap.
func Copy(dst []byte, dstStart int, src []byte, srcStart, n int) {
	for n > 0 {
		k := min(n, 32)
		SetField(dst, dstStart, k, Field(src, srcStart, k))
		dstStart += k
		srcStart += k
		n -= k
	}
}

// Unpack writes len(dst) bits of src from bit start into dst, one bit.Bit
// per bit.
func Unpack(dst []bit.Bit, src []byte, start int) {
	if start&7 == 0 {
		for i := range dst {
			dst[i] = bit.Bit(src[start>>3+i>>3] >> (7 - i&7) & 1)
		}
		return
	}
	for i := range dst {
		dst[i] = bit.Bit(Bit(src, start+i))
	}
}

// Pack writes the low bits of src into dst from bit start. The other bits of
// dst stay as they are.
func Pack(dst []byte, start int, src []bit.Bit) {
	if start&7 == 0 {
		whole := len(src) &^ 7
		for i := 0; i < whole; i += 8 {
			var x byte
			for _, b := range src[i : i+8] {
				x = x<<1 | byte(b&1)
			}
			dst[start>>3+i>>3] = x
		}
		src, start = src[whole:], start+whole
	}
	for i, b := range src {
		SetBit(dst, start+i, byte(b))
	}
}
