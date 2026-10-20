package crc

const crc7Poly = 0x27 // x⁵ + x² + x + 1 (low 7 bits of G₇(x))

// CRC7 computes the CRC-7 of the first n bits of data, the first bit most
// significant, with the generator polynomial x⁷ + x⁵ + x² + x + 1, initial
// value 0 and no final inversion. The caller applies the CRC mask of clause
// B.3.12. (ETSI TS 102 361-1 clause B.3.13)
func CRC7(data []byte, n int) uint8 {
	return crc8Reg(data, n, &crc7_0xA7, crc7Poly<<1) >> 1
}

// CheckCRC7 reports whether want is the CRC-7 of the first n bits of data.
func CheckCRC7(data []byte, n int, want uint8) bool {
	return CRC7(data, n) == want&0x7F
}

// crc8Reg runs the first n bits of data through a CRC register of up to 8
// bits that holds the CRC in its top bits, with the byte table and the
// polynomial of that register, and returns the register.
func crc8Reg(data []byte, n int, table *[256]uint8, poly uint8) uint8 {
	var reg uint8
	for _, b := range data[:n>>3] {
		reg = table[reg^b]
	}
	if rest := n & 7; rest != 0 {
		b := data[n>>3]
		for range rest {
			feedback := (reg ^ b) & 0x80
			reg <<= 1
			b <<= 1
			if feedback != 0 {
				reg ^= poly
			}
		}
	}
	return reg
}
