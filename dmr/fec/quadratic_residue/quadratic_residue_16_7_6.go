package quadratic_residue

import (
	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - B.3.2  Quadratic residue (16,7,6)

type QuadraticResidue16_7_6 struct {
	Data   [7]bit.Bit
	Parity [9]bit.Bit
}

func NewQuadraticResidue16_7_6(bits [16]bit.Bit) *QuadraticResidue16_7_6 {
	qr := QuadraticResidue16_7_6{}
	copy(qr.Data[:], bits[:7])
	copy(qr.Parity[:], bits[7:16])

	return &qr
}

func ParityBits(bits [7]bit.Bit) [9]bit.Bit {
	parity := [9]bit.Bit{}

	// Multiplying the generator matrix with the given data bits.
	// See DMR AI spec. page 134.
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

func Check(bits [16]bit.Bit) bool {
	var data [7]bit.Bit
	copy(data[:], bits[:7])

	calcParity := ParityBits(data)
	for i := 0; i < 9; i++ {
		if calcParity[i] != bits[7+i] {
			return false
		}
	}
	return true
}

// Decode corrects up to 2 bit errors in the 16-bit codeword.
// Returns the corrected data words and FECResult.
func Decode(bits [16]bit.Bit) ([16]bit.Bit, fec.FECResult) {
	result := fec.FECResult{BitsChecked: 16}

	var data [7]bit.Bit
	copy(data[:], bits[:7])

	// Calculate syndrome
	calcParity := ParityBits(data)
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

	// Apply correction (error pattern: 1 = flip)
	var corrected [16]bit.Bit
	copy(corrected[:], bits[:])

	for i := 0; i < 16; i++ {
		if (errPattern & (1 << (15 - i))) != 0 {
			corrected[i] ^= 1
			result.ErrorsCorrected++
		}
	}

	return corrected, result
}

// Encode encodes a 7-bit input into a 16-bit QR(16,7,6) codeword.
// The input byte contains the 7 data bits (0..127).
// The output array contains 16 bits (values 0 or 1).
// Bit layout: bits[0..6] = data, bits[7..15] = parity.
func Encode(data byte) [16]bit.Bit {
	var bits [16]bit.Bit

	// Look up the packed 16-bit codeword from the encoding table.
	// Table layout: bits 15..9 = data[0..6], bits 8..0 = parity[0..8].
	codeword := qr16_7_6_encoding_table[data&0x7F]

	// Unpack into bit array
	for i := 0; i < 16; i++ {
		if (codeword>>(15-i))&1 == 1 {
			bits[i] = 1
		}
	}

	return bits
}
