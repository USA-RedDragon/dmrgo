package crc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// TS 102 361-1 - B.3.7  8-bit CRC calculation
//
// Generator polynomial: G₈(x) = x⁸ + x² + x + 1 (0x107)
// Initial remainder: 0x00
// Final XOR: none
//
// F₈(x) = x⁸ · M(x) mod G₈(x)

const crc8Poly = 0x07 // x² + x + 1 (low 8 bits of G₈(x))

// CRC8 computes the 8-bit CRC over the given bit data.
// The polynomial is x⁸ + x² + x + 1, initial value 0, no final inversion.
func CRC8(bits []bit.Bit) uint8 {
	var crc uint8

	for _, b := range bits {
		feedback := ((crc >> 7) & 1) ^ uint8(b&1)
		crc <<= 1
		if feedback != 0 {
			crc ^= crc8Poly
		}
	}

	return crc
}

// CheckCRC8 reports whether want is the CRC-8 of bits.
func CheckCRC8(bits []bit.Bit, want uint8) bool {
	return CRC8(bits) == want
}
