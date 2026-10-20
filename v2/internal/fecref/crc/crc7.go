package crc

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

const crc7Poly = 0x27

// CRC7 computes the CRC-7 of bits with the generator polynomial x⁷ + x⁵ + x²
// + x + 1, initial value 0 and no final inversion. The caller applies the CRC
// mask of clause B.3.12. (ETSI TS 102 361-1 clause B.3.13)
func CRC7(bits []unpacked.Bit) uint8 {
	var crc uint8

	for _, b := range bits {
		feedback := ((crc >> 6) & 1) ^ uint8(b&1)
		crc = (crc << 1) & 0x7F
		if feedback != 0 {
			crc ^= crc7Poly
		}
	}

	return crc
}

// CheckCRC7 reports whether want is the CRC-7 of bits.
func CheckCRC7(bits []unpacked.Bit, want uint8) bool {
	return CRC7(bits) == want&0x7F
}
