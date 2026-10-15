// Package golay implements the Golay (20,8,7) code of ETSI TS 102 361-1
// B.3.1, which protects the slot type, and the Golay (23,12,7) and (24,12,8)
// codes of the AMBE+2 vocoder frame FEC.
package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// parity2087 returns the 12 parity bits of 8 data bits in the Golay (20,8,7)
// code of ETSI TS 102 361-1 B.3.1.
func parity2087(bits [8]bit.Bit) [12]bit.Bit {
	bit.Mask(bits[:])
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

// Decode2087 corrects up to 3 bit errors in a Golay (20,8,7) codeword and
// returns the corrected codeword and a fec.Result.
func Decode2087(bits [20]bit.Bit) ([20]bit.Bit, fec.Result) {
	bit.Mask(bits[:])
	result := fec.Result{BitsChecked: 20}

	var data [8]bit.Bit
	copy(data[:], bits[:8])

	// Calculate syndrome
	calcParity := parity2087(data)
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

// Check2087 reports whether bits is a Golay (20,8,7) codeword.
func Check2087(bits [20]bit.Bit) bool {
	bit.Mask(bits[:])
	var dataBits [8]bit.Bit
	copy(dataBits[:], bits[:8])

	parity := parity2087(dataBits)
	for i := 0; i < 12; i++ {
		if parity[i] != bits[8+i] {
			return false
		}
	}
	return true
}

// Encode2087 returns the Golay (20,8,7) codeword of 8 data bits: the data
// bits followed by 12 parity bits.
func Encode2087(dataBits [8]bit.Bit) [20]bit.Bit {
	var result [20]bit.Bit
	data := uint8(bit.Uint(dataBits[:8])) //nolint:gosec // 8 bits

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
