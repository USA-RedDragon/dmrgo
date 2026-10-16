// Package hamming encodes and decodes the Hamming (7,4,3) code that protects
// the TACT of the CACH (ETSI TS 102 361-1 clause B.3.5).
package hamming

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// parity743 returns the 3 Hamming(7,4,3) parity bits of 4 data bits, from
// the generator matrix [I₄ | P] of ETSI TS 102 361-1 clause B.3.5.
func parity743(data [4]unpacked.Bit) [3]unpacked.Bit {
	unpacked.Mask(data[:])
	return [3]unpacked.Bit{
		data[0] ^ data[1] ^ data[2],
		data[1] ^ data[2] ^ data[3],
		data[0] ^ data[1] ^ data[3],
	}
}

// Decode743 corrects up to 1 bit error in a 7-bit Hamming(7,4,3) codeword
// and returns the corrected codeword and a fec.Result.
func Decode743(bits [7]unpacked.Bit) ([7]unpacked.Bit, fec.Result) {
	unpacked.Mask(bits[:])
	result := fec.Result{BitsChecked: 7}

	s0 := bits[0] ^ bits[1] ^ bits[2] ^ bits[4]
	s1 := bits[1] ^ bits[2] ^ bits[3] ^ bits[5]
	s2 := bits[0] ^ bits[1] ^ bits[3] ^ bits[6]

	syndrome := int(s0)<<2 | int(s1)<<1 | int(s2)

	if syndrome == 0 {
		return bits, result
	}

	errPos := hamming743_syndrome_table[syndrome]
	if errPos < 0 {
		// Should not happen for a 3-bit syndrome in Hamming(7,4,3)
		result.Uncorrectable = true
		return bits, result
	}

	corrected := bits
	corrected[errPos] ^= 1
	result.ErrorsCorrected = 1

	return corrected, result
}

// Check743 reports whether bits is a Hamming(7,4,3) codeword.
func Check743(bits [7]unpacked.Bit) bool {
	unpacked.Mask(bits[:])
	var data [4]unpacked.Bit
	copy(data[:], bits[:4])

	parity := parity743(data)
	return parity[0] == bits[4] && parity[1] == bits[5] && parity[2] == bits[6]
}

// Encode743 encodes 4 data bits into a 7-bit Hamming(7,4,3) codeword: the
// data bits followed by the 3 parity bits.
func Encode743(data [4]unpacked.Bit) [7]unpacked.Bit {
	unpacked.Mask(data[:])
	parity := parity743(data)
	return [7]unpacked.Bit{
		data[0], data[1], data[2], data[3],
		parity[0], parity[1], parity[2],
	}
}
