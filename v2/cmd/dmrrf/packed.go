package main

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// cachBytes packs the 24 bits of a CACH field, one bit.Bit per bit, in the
// form that the layer2 CACH functions take.
func cachBytes(b [layer2.CACHBits]bit.Bit) [3]byte {
	var p [3]byte
	for i, v := range b {
		p[i>>3] |= byte(v&1) << (7 - i&7)
	}
	return p
}

// cachBits is the inverse of cachBytes.
func cachBits(p [3]byte) [layer2.CACHBits]bit.Bit {
	var b [layer2.CACHBits]bit.Bit
	for i := range b {
		b[i] = bit.Bit(p[i>>3] >> (7 - i&7) & 1)
	}
	return b
}

// cachPayload returns the 17-bit CACH payload word f packed as
// layer2.CACH.Payload holds it.
func cachPayload(f uint32) [3]byte {
	p := f << 7
	return [3]byte{byte(p >> 16 & 0xFF), byte(p >> 8 & 0xFF), byte(p & 0x80)}
}

// cachPayloadWord is the inverse of cachPayload.
func cachPayloadWord(p [3]byte) uint32 {
	return uint32(p[0])<<9 | uint32(p[1])<<1 | uint32(p[2])>>7
}
