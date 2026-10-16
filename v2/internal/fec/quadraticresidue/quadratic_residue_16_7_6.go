// Package quadraticresidue encodes and decodes the quadratic residue
// (16,7,6) code that protects the EMB (ETSI TS 102 361-1 clause B.3.2).
package quadraticresidue

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// Encode returns the QR (16,7,6) codeword of the low 7 bits of data: the
// data bits followed by 9 parity bits, the first bit most significant.
func Encode(data uint8) uint16 {
	return qr16_7_6_encoding_table[data&0x7F]
}

// Check reports whether cw is a QR (16,7,6) codeword.
func Check(cw uint16) bool {
	return Encode(uint8(cw>>9)) == cw
}

// Decode corrects up to 2 bit errors in the QR (16,7,6) codeword cw, the
// first bit most significant, and returns the corrected codeword and a
// fec.Result. An uncorrectable codeword is returned as it is.
func Decode(cw uint16) (uint16, fec.Result) {
	result := fec.Result{BitsChecked: 16}
	syndrome := (Encode(uint8(cw>>9)) ^ cw) & 0x1FF
	if syndrome == 0 {
		return cw, result
	}
	errPattern := qr16_7_6_syndrome_table[syndrome]
	if errPattern == 0xFFFF {
		result.Uncorrectable = true
		return cw, result
	}
	result.ErrorsCorrected = bits.OnesCount16(errPattern)
	return cw ^ errPattern, result
}
