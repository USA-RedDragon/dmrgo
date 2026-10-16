package crc

const crc7Poly = 0x27 // x⁵ + x² + x + 1 (low 7 bits of G₇(x))

// CRC7 computes the CRC-7 of the first n bits of data, the first bit most
// significant, with the generator polynomial x⁷ + x⁵ + x² + x + 1, initial
// value 0 and no final inversion (ETSI TS 102 361-1 clause B.3.13). The
// caller applies the CRC mask of clause B.3.12.
func CRC7(data []byte, n int) uint8 {
	var crc uint8
	for i := range n {
		feedback := crc>>6&1 ^ data[i>>3]>>(7-i&7)&1
		crc = crc << 1 & 0x7F
		if feedback != 0 {
			crc ^= crc7Poly
		}
	}
	return crc
}

// CheckCRC7 reports whether want is the CRC-7 of the first n bits of data.
func CheckCRC7(data []byte, n int, want uint8) bool {
	return CRC7(data, n) == want&0x7F
}
