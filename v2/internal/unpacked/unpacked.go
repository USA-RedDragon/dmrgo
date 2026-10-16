// Package unpacked converts between packed bytes and bits held one bit.Bit
// per bit, the first bit most significant, for the bit-per-bit reference
// codecs and the tests. The library API takes packed bytes. A
// function panics when a slice is shorter than the bits it reads or writes.
package unpacked

import "github.com/USA-RedDragon/dmrgo/v2/bit"

// Uint returns bits as an unsigned integer, the first bit most significant.
// It reads at most the last 64 bits of a longer slice.
func Uint(bits []bit.Bit) uint64 {
	var v uint64
	for _, b := range bits {
		v = v<<1 | uint64(b&1)
	}
	return v
}

// PutUint writes the low len(dst) bits of v into dst, the most significant
// of them first. Bits of dst beyond the 64th from the end are set to 0.
func PutUint(dst []bit.Bit, v uint64) {
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i] = bit.Bit(v & 1)
		v >>= 1
	}
}

// Pack packs bits into bytes, the first bit in the most significant bit of
// the first byte. The last byte is padded with 0 bits. It allocates the
// slice; AppendPack does not.
func Pack(bits []bit.Bit) []byte {
	return AppendPack(make([]byte, 0, (len(bits)+7)/8), bits)
}

// AppendPack appends the bytes that Pack returns for bits to dst and
// returns the extended slice. It allocates only when dst lacks the capacity.
func AppendPack(dst []byte, bits []bit.Bit) []byte {
	for i := 0; i < len(bits); i += 8 {
		var b byte
		for j, x := range bits[i:min(i+8, len(bits))] {
			b |= byte(x&1) << (7 - j)
		}
		dst = append(dst, b)
	}
	return dst
}

// Unpack is the reverse of Pack: it returns the len(data)*8 bits of data.
// It allocates the slice; AppendUnpack does not.
func Unpack(data []byte) []bit.Bit {
	return AppendUnpack(make([]bit.Bit, 0, len(data)*8), data)
}

// AppendUnpack appends the bits that Unpack returns for data to dst and
// returns the extended slice. It allocates only when dst lacks the capacity.
func AppendUnpack(dst []bit.Bit, data []byte) []bit.Bit {
	for _, b := range data {
		dst = append(dst, bit.Bit(b>>7&1), bit.Bit(b>>6&1), bit.Bit(b>>5&1), bit.Bit(b>>4&1), bit.Bit(b>>3&1), bit.Bit(b>>2&1), bit.Bit(b>>1&1), bit.Bit(b&1))
	}
	return dst
}

// Unpack32 returns the 32 bits of 4 bytes, such as an embedded signalling
// field, as Unpack does.
func Unpack32(data [4]byte) [32]bit.Bit {
	var bits [32]bit.Bit
	for i := range 32 {
		if (data[i/8]>>(7-(i%8)))&1 != 0 {
			bits[i] = 1
		}
	}
	return bits
}

// Pack32 packs 32 bits, such as an embedded signalling field, into 4 bytes,
// as Pack does.
func Pack32(bits [32]bit.Bit) [4]byte {
	var data [4]byte
	for i := range 32 {
		if bits[i]&1 == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}

// Unpack96 returns the 96 bits of a 12-byte BPTC(196,96) payload, as Unpack
// does.
func Unpack96(data [12]byte) [96]bit.Bit {
	var bits [96]bit.Bit
	for i := range 96 {
		if (data[i/8]>>(7-(i%8)))&1 != 0 {
			bits[i] = 1
		}
	}
	return bits
}

// Pack96 packs 96 bits, such as the information bits of a CSBK or data
// header, into 12 bytes, as Pack does.
func Pack96(bits [96]bit.Bit) [12]byte {
	var data [12]byte
	for i := range 96 {
		if bits[i]&1 == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}
