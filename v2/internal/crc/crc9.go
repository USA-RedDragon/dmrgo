package crc

const (
	crc9Poly      = uint16(0x59)  // x⁶ + x⁴ + x³ + 1 (low 9 bits of G₉(x) without x⁹)
	crc9Inversion = uint16(0x1FF) // all 9 bits set
)

// CRC9 computes the CRC-9 of the first n bits of data, the first bit most
// significant, with the generator polynomial x⁹ + x⁶ + x⁴ + x³ + 1, initial
// value 0 and the result inverted with 0x1FF (ETSI TS 102 361-1 clause
// B.3.10).
func CRC9(data []byte, n int) uint16 {
	// The register holds the 9 CRC bits in its top bits, so that a whole
	// byte goes through the table at once.
	var reg uint16
	for _, b := range data[:n>>3] {
		reg = reg<<8 ^ crc9_0x259[byte(reg>>8)^b]
	}
	if rest := n & 7; rest != 0 {
		b := uint16(data[n>>3]) << 8
		for range rest {
			feedback := (reg ^ b) & 0x8000
			reg <<= 1
			b <<= 1
			if feedback != 0 {
				reg ^= crc9Poly << 7
			}
		}
	}
	return reg>>7 ^ crc9Inversion
}

// CheckCRC9 reports whether want is the CRC-9 of the first n bits of data.
func CheckCRC9(data []byte, n int, want uint16) bool {
	return CRC9(data, n) == want&0x1FF
}
