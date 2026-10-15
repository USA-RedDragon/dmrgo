package crc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// TS 102 361-1 - B.3.13  7-bit CRC calculation
//
// Generator polynomial: G₇(x) = x⁷ + x⁵ + x² + x + 1 (0xA7)
// Initial remainder: 0x00
// Final XOR: none (masks are applied separately per B.3.12)
//
// F₇(x) = x⁷ · M(x) mod G₇(x)

const crc7Poly = 0x27 // x⁵ + x² + x + 1 (low 7 bits of G₇(x))

// CRC7 computes the 7-bit CRC over the given bit data.
// The polynomial is x⁷ + x⁵ + x² + x + 1, initial value 0, no final inversion.
// Data type CRC masks (per B.3.12) must be applied by the caller if needed.
func CRC7(bits []bit.Bit) uint8 {
	var crc uint8

	for _, b := range bits {
		feedback := ((crc >> 6) & 1) ^ uint8(b&1)
		crc = (crc << 1) & 0x7F // keep only 7 bits
		if feedback != 0 {
			crc ^= crc7Poly
		}
	}

	return crc
}

// CheckCRC7 reports whether want is the CRC-7 of bits.
func CheckCRC7(bits []bit.Bit, want uint8) bool {
	return CRC7(bits) == want&0x7F
}
