package crc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// TS 102 361-1 - B.3.10  CRC-9 calculation
//
// Generator polynomial: G₉(x) = x⁹ + x⁶ + x⁴ + x³ + 1 (0x259)
// Initial remainder: 0x000
// Inversion polynomial: I₉(x) = x⁸ + x⁷ + ... + x + 1 = 0x1FF
//
// F₉(x) = (x⁹ · M(x) mod G₉(x)) + I₉(x)
//
// The result is XORed with 0x1FF after polynomial division.

const (
	crc9Poly      = uint16(0x59)  // x⁶ + x⁴ + x³ + 1 (low 9 bits of G₉(x) without x⁹)
	crc9Inversion = uint16(0x1FF) // all 9 bits set
)

// CRC9 computes the 9-bit CRC over the given bit data.
// The polynomial is x⁹ + x⁶ + x⁴ + x³ + 1, with final inversion by 0x1FF.
func CRC9(bits []bit.Bit) uint16 {
	var crc uint16

	for _, b := range bits {
		feedback := ((crc >> 8) & 1) ^ uint16(b&1)
		crc = (crc << 1) & 0x1FF // keep only 9 bits
		if feedback != 0 {
			crc ^= crc9Poly
		}
	}

	return crc ^ crc9Inversion
}

// CheckCRC9 reports whether want is the CRC-9 of bits.
func CheckCRC9(bits []bit.Bit, want uint16) bool {
	return CRC9(bits) == want&0x1FF
}
