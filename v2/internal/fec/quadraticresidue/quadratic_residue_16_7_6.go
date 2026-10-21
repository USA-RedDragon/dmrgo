// Package quadraticresidue encodes and decodes the quadratic residue
// (16,7,6) code that protects the EMB (ETSI TS 102 361-1 clause B.3.2).
package quadraticresidue

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

var rows = [8]uint16{0x0273, 0x04e5, 0x09c9, 0x11e2, 0x21b7, 0x411e, 0x804f}

// Encode returns the QR (16,7,6) codeword of the low 7 bits of data: the
// data bits followed by 9 parity bits, the first bit most significant.
func Encode(data uint8) uint16 {
	var cw uint16
	for k := range 7 {
		cw ^= rows[k] & -uint16(data>>k&1)
	}
	return cw
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
	if c := Encode(uint8(cw >> 9)); bits.OnesCount16(c^cw) <= 2 {
		result.ErrorsCorrected = bits.OnesCount16(c ^ cw)
		return c, result
	}
	var c uint16
	for i := 1; ; i++ {
		if e := bits.OnesCount16(c ^ cw); e <= 2 {
			result.ErrorsCorrected = e
			return c, result
		}
		if i == 128 {
			break
		}
		c ^= rows[bits.TrailingZeros(uint(i))&7]
	}
	result.Uncorrectable = true
	return cw, result
}
