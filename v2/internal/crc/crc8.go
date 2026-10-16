package crc

const crc8Poly = 0x07 // x² + x + 1 (low 8 bits of G₈(x))

// CRC8 computes the CRC-8 of the first n bits of data, the first bit most
// significant, with the generator polynomial x⁸ + x² + x + 1, initial value
// 0 and no final inversion (ETSI TS 102 361-1 clause B.3.7).
func CRC8(data []byte, n int) uint8 {
	var crc uint8
	for i := range n {
		feedback := crc>>7 ^ data[i>>3]>>(7-i&7)&1
		crc <<= 1
		if feedback != 0 {
			crc ^= crc8Poly
		}
	}
	return crc
}

// CheckCRC8 reports whether want is the CRC-8 of the first n bits of data.
func CheckCRC8(data []byte, n int, want uint8) bool {
	return CRC8(data, n) == want
}
