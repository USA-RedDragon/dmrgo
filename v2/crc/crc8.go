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

// CalculateCRC8 computes the 8-bit CRC over the given bit data.
// The polynomial is x⁸ + x² + x + 1, initial value 0, no final inversion.
func CalculateCRC8(bits []bit.Bit) uint8 {
	var crc uint8

	for _, b := range bits {
		feedback := ((crc >> 7) & 1) ^ uint8(b)
		crc <<= 1
		if feedback != 0 {
			crc ^= crc8Poly
		}
	}

	return crc
}

// CheckCRC8 validates an 8-bit CRC. The input should contain the data bits
// followed by the 8-bit CRC. Returns true if the CRC is valid.
func CheckCRC8(bits []bit.Bit) bool {
	if len(bits) < 9 {
		return false
	}

	dataBits := bits[:len(bits)-8]
	crc := CalculateCRC8(dataBits)

	// Extract expected CRC from trailing 8 bits
	var expected uint8
	for i := 0; i < 8; i++ {
		expected <<= 1
		expected |= uint8(bits[len(bits)-8+i])
	}

	return crc == expected
}
