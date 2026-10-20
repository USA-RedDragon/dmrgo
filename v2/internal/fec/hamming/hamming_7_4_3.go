// Package hamming encodes and decodes the Hamming (7,4,3) code that protects
// the TACT of the CACH (ETSI TS 102 361-1 clause B.3.5).
package hamming

import "github.com/USA-RedDragon/dmrgo/v2/fec"

// ETSI TS 102 361-1 clause B.3.5
const (
	check0 = 0b1110100
	check1 = 0b0111010
	check2 = 0b1101001
)

func parity(x uint8) uint8 {
	x ^= x >> 4
	x ^= x >> 2
	x ^= x >> 1
	return x & 1
}

func syndrome(cw uint8) uint8 {
	return parity(cw&check0)<<2 | parity(cw&check1)<<1 | parity(cw&check2)
}

// Encode743 returns the Hamming (7,4,3) codeword of the low 4 bits of data:
// the data bits followed by 3 parity bits, the first bit most significant.
func Encode743(data uint8) uint8 {
	cw := (data & 0xF) << 3
	return cw | syndrome(cw)
}

// Check743 reports whether the low 7 bits of cw are a Hamming (7,4,3)
// codeword.
func Check743(cw uint8) bool {
	return syndrome(cw&0x7F) == 0
}

// Decode743 corrects up to 1 bit error in the 7-bit Hamming (7,4,3)
// codeword cw, the first bit most significant, and returns the corrected
// codeword and a fec.Result. Bits of cw above the 7th are ignored.
func Decode743(cw uint8) (uint8, fec.Result) {
	cw &= 0x7F
	result := fec.Result{BitsChecked: 7}
	s := syndrome(cw)
	if s == 0 {
		return cw, result
	}
	result.ErrorsCorrected = 1
	return cw ^ 0x40>>hamming743_syndrome_table[s], result
}
