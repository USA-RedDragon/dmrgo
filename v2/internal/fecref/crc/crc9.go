package crc

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

const (
	crc9Poly      = uint16(0x59)  // x⁶ + x⁴ + x³ + 1 (low 9 bits of G₉(x) without x⁹)
	crc9Inversion = uint16(0x1FF) // all 9 bits set
)

// CRC9 computes the CRC-9 of bits with the generator polynomial
// x⁹ + x⁶ + x⁴ + x³ + 1, initial value 0 and the result inverted with 0x1FF
// (ETSI TS 102 361-1 clause B.3.10).
func CRC9(bits []unpacked.Bit) uint16 {
	var crc uint16

	for _, b := range bits {
		feedback := ((crc >> 8) & 1) ^ uint16(b&1)
		crc = (crc << 1) & 0x1FF
		if feedback != 0 {
			crc ^= crc9Poly
		}
	}

	return crc ^ crc9Inversion
}

// CheckCRC9 reports whether want is the CRC-9 of bits.
func CheckCRC9(bits []unpacked.Bit, want uint16) bool {
	return CRC9(bits) == want&0x1FF
}
