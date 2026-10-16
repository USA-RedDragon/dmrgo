package crc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

const crc8Poly = 0x07 // x² + x + 1 (low 8 bits of G₈(x))

// CRC8 computes the CRC-8 of bits with the generator polynomial
// x⁸ + x² + x + 1, initial value 0 and no final inversion (ETSI TS 102 361-1
// clause B.3.7).
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
