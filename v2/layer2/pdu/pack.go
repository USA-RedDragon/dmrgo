package pdu

import "github.com/USA-RedDragon/dmrgo/v2/bit"

// unpackBits writes the bits of src into dst, the most significant bit of
// each byte first, as bit.Unpack does, and stops at the end of dst.
func unpackBits(dst []bit.Bit, src []byte) {
	for i := range dst {
		dst[i] = bit.Bit(src[i/8] >> (7 - i%8) & 1)
	}
}

// packBits packs src into dst, the first bit in the most significant bit of
// the first byte, as bit.Pack does. dst must start zero.
func packBits(dst []byte, src []bit.Bit) {
	for i, b := range src {
		dst[i/8] |= byte(b&1) << (7 - i%8)
	}
}
