// Package golay encodes and decodes the Golay (20,8,7) code that protects
// the slot type (ETSI TS 102 361-1 clause B.3.1), and the Golay (23,12,7) and
// (24,12,8) codes of the AMBE+2 vocoder frame FEC.
package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

var rows2087 = [8]uint32{0x018eb, 0x0293e, 0x04a97, 0x08dc6, 0x10367, 0x206cd, 0x40d99, 0x803da}

// Encode2087 returns the Golay (20,8,7) codeword of 8 data bits: the data
// bits followed by 12 parity bits, the first bit most significant.
func Encode2087(data uint8) uint32 {
	var cw uint32
	for k := range 8 {
		cw ^= rows2087[k] & -uint32(data>>k&1)
	}
	return cw
}

// Decode2087 corrects up to 3 bit errors in the 20-bit Golay (20,8,7)
// codeword cw, the first bit most significant, and returns the corrected
// codeword and a fec.Result. Bits of cw above the 20th are ignored. An
// uncorrectable codeword is returned as it is.
func Decode2087(cw uint32) (uint32, fec.Result) {
	cw &= 0xFFFFF
	result := fec.Result{BitsChecked: 20}
	if c := Encode2087(uint8(cw >> 12)); bits.OnesCount32(c^cw) <= 3 {
		result.ErrorsCorrected = bits.OnesCount32(c ^ cw)
		return c, result
	}
	var c uint32
	for i := 1; ; i++ {
		if e := bits.OnesCount32(c ^ cw); e <= 3 {
			result.ErrorsCorrected = e
			return c, result
		}
		if i == 256 {
			break
		}
		c ^= rows2087[bits.TrailingZeros(uint(i))&7]
	}
	result.Uncorrectable = true
	return cw, result
}

// Check2087 reports whether the low 20 bits of cw are a Golay (20,8,7)
// codeword.
func Check2087(cw uint32) bool {
	cw &= 0xFFFFF
	return Encode2087(uint8(cw>>12)) == cw
}
