package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - B.3.1 Golay (20,8,7)
// This code works on Bit arrays where each Bit is 0 or 1.
func Golay_20_8_Parity(bits [8]bit.Bit) [12]bit.Bit {
	var p [12]bit.Bit
	p[0] = bits[1] ^ bits[4] ^ bits[5] ^ bits[6] ^ bits[7]
	p[1] = bits[1] ^ bits[2] ^ bits[4]
	p[2] = bits[0] ^ bits[2] ^ bits[3] ^ bits[5]
	p[3] = bits[0] ^ bits[1] ^ bits[3] ^ bits[4] ^ bits[6]
	p[4] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[7]
	p[5] = bits[0] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[7]
	p[6] = bits[3] ^ bits[6] ^ bits[7]
	p[7] = bits[0] ^ bits[1] ^ bits[5] ^ bits[6]
	p[8] = bits[0] ^ bits[1] ^ bits[2] ^ bits[6] ^ bits[7]
	p[9] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6]
	p[10] = bits[0] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6] ^ bits[7]
	p[11] = bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7]
	return p
}

// DecodeGolay2087 decodes a 20-bit sequence (Bit array of 0s and 1s)
// Returns corrected bits and FECResult.
func DecodeGolay2087(bits [20]bit.Bit) ([20]bit.Bit, fec.FECResult) {
	result := fec.FECResult{BitsChecked: 20}

	var data [8]bit.Bit
	copy(data[:], bits[:8])

	// Calculate syndrome
	calcParity := Golay_20_8_Parity(data)
	syndrome := 0
	for i := 0; i < 12; i++ {
		if calcParity[i] != bits[8+i] {
			syndrome |= (1 << i)
		}
	}

	if syndrome == 0 {
		return bits, result
	}

	errPattern := golay_20_8_7_syndrome_table[syndrome]
	if errPattern == 0xFFFFFFFF {
		result.Uncorrectable = true
		return bits, result
	}

	// Apply correction
	var corrected [20]bit.Bit
	copy(corrected[:], bits[:])

	for i := 0; i < 20; i++ {
		if (errPattern & (1 << i)) != 0 {
			corrected[i] ^= 1
			result.ErrorsCorrected++
		}
	}

	return corrected, result
}

func Golay_20_8_Check(bits [20]bit.Bit) bool {
	var dataBits [8]bit.Bit
	copy(dataBits[:], bits[:8])

	parity := Golay_20_8_Parity(dataBits)
	for i := 0; i < 12; i++ {
		if parity[i] != bits[8+i] {
			return false
		}
	}
	return true
}

// Encode encodes an 8-bit input into a 20-bit Golay(20,8,7) codeword.
// The input byte contains the 8 data bits.
// The output array contains 20 bits (values 0 or 1).
func Encode(data byte) [20]bit.Bit {
	var result [20]bit.Bit

	// The encoding table uses LSB-first bit ordering (bits[i] = (d>>i)&1),
	// but the codeword bit array uses MSB-first (bits[i] = (data>>(7-i))&1).
	// Reverse the data byte to match the table's convention.
	tableIdx := bits.Reverse8(data)
	codeword := golay_20_8_7_table[tableIdx]

	// Unpack data bits (MSB first)
	for i := 0; i < 8; i++ {
		if (data>>(7-i))&1 == 1 {
			result[i] = 1
		}
	}
	// Unpack parity bits from table
	for i := 0; i < 12; i++ {
		if (codeword>>(8+i))&1 == 1 {
			result[8+i] = 1
		}
	}

	return result
}
