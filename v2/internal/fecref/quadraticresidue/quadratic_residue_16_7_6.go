// Package quadraticresidue encodes and decodes the quadratic residue
// (16,7,6) code that protects the EMB (ETSI TS 102 361-1 clause B.3.2).
package quadraticresidue

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// parityBits returns the 9 parity bits of 7 data bits, from the generator
// matrix of ETSI TS 102 361-1 clause B.3.2.
func parityBits(bits [7]unpacked.Bit) [9]unpacked.Bit {
	unpacked.Mask(bits[:])
	parity := [9]unpacked.Bit{}

	parity[0] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4]
	parity[1] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5]
	parity[2] = bits[0] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6]
	parity[3] = bits[2] ^ bits[3] ^ bits[5] ^ bits[6]
	parity[4] = bits[1] ^ bits[2] ^ bits[6]
	parity[5] = bits[0] ^ bits[1] ^ bits[4]
	parity[6] = bits[0] ^ bits[1] ^ bits[2] ^ bits[5]
	parity[7] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[6]
	parity[8] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[6]

	return parity
}

// Check reports whether bits is a QR (16,7,6) codeword.
func Check(bits [16]unpacked.Bit) bool {
	unpacked.Mask(bits[:])
	var data [7]unpacked.Bit
	copy(data[:], bits[:7])

	calcParity := parityBits(data)
	for i := 0; i < 9; i++ {
		if calcParity[i] != bits[7+i] {
			return false
		}
	}
	return true
}

// Decode corrects up to 2 bit errors in a QR (16,7,6) codeword and returns
// the corrected codeword and a fec.Result.
func Decode(bits [16]unpacked.Bit) ([16]unpacked.Bit, fec.Result) {
	unpacked.Mask(bits[:])
	result := fec.Result{BitsChecked: 16}

	var data [7]unpacked.Bit
	copy(data[:], bits[:7])

	calcParity := parityBits(data)
	syndrome := 0
	for i := 0; i < 9; i++ {
		if calcParity[i] != bits[7+i] {
			syndrome |= (1 << (8 - i))
		}
	}

	if syndrome == 0 {
		return bits, result
	}

	errPattern := qr16_7_6_syndrome_table[syndrome]
	if errPattern == 0xFFFF {
		result.Uncorrectable = true
		return bits, result
	}

	var corrected [16]unpacked.Bit
	copy(corrected[:], bits[:])

	for i := 0; i < 16; i++ {
		if (errPattern & (1 << (15 - i))) != 0 {
			corrected[i] ^= 1
			result.ErrorsCorrected++
		}
	}

	return corrected, result
}

// Encode returns the QR (16,7,6) codeword of 7 data bits: the data bits
// followed by 9 parity bits.
func Encode(dataBits [7]unpacked.Bit) [16]unpacked.Bit {
	var bits [16]unpacked.Bit
	data := uint8(unpacked.Uint(dataBits[:7]) & 0xFF)

	// Bits 15 to 9 of a table entry are data[0] to data[6], and bits 8 to 0
	// are parity[0] to parity[8].
	codeword := qr16_7_6_encoding_table[data&0x7F]

	for i := 0; i < 16; i++ {
		if (codeword>>(15-i))&1 == 1 {
			bits[i] = 1
		}
	}

	return bits
}
