// Package golay encodes and decodes the Golay (20,8,7) code that protects
// the slot type (ETSI TS 102 361-1 clause B.3.1), and the Golay (23,12,7) and
// (24,12,8) codes of the AMBE+2 vocoder frame FEC.
package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// reverse20 reverses the order of the low 20 bits of x. The Golay (20,8,7)
// tables hold codeword bit i in bit i, and a packed codeword holds it in bit
// 19-i.
func reverse20(x uint32) uint32 {
	return bits.Reverse32(x) >> 12
}

// Encode2087 returns the Golay (20,8,7) codeword of 8 data bits: the data
// bits followed by 12 parity bits, the first bit most significant.
func Encode2087(data uint8) uint32 {
	return reverse20(golay_20_8_7_table[bits.Reverse8(data)])
}

// Decode2087 corrects up to 3 bit errors in the 20-bit Golay (20,8,7)
// codeword cw, the first bit most significant, and returns the corrected
// codeword and a fec.Result. Bits of cw above the 20th are ignored. An
// uncorrectable codeword is returned as it is.
func Decode2087(cw uint32) (uint32, fec.Result) {
	cw &= 0xFFFFF
	result := fec.Result{BitsChecked: 20}
	diff := (Encode2087(uint8(cw>>12)) ^ cw) & 0xFFF
	if diff == 0 {
		return cw, result
	}
	errPattern := golay_20_8_7_syndrome_table[bits.Reverse16(uint16(diff))>>4]
	if errPattern == 0xFFFFFFFF {
		result.Uncorrectable = true
		return cw, result
	}
	result.ErrorsCorrected = bits.OnesCount32(errPattern)
	return cw ^ reverse20(errPattern), result
}

// Check2087 reports whether the low 20 bits of cw are a Golay (20,8,7)
// codeword.
func Check2087(cw uint32) bool {
	cw &= 0xFFFFF
	return Encode2087(uint8(cw>>12)) == cw
}
