package crc

// CRC32 computes the CRC-32 of data with the generator polynomial
// 0x04C11DB7, initial value 0 and no final inversion. It swaps the two bytes
// of each 16-bit word before the division, and a last odd byte stays in place
// (ETSI TS 102 361-1 clause B.3.9).
func CRC32(data []byte) uint32 {
	var crc uint32
	for i := range data {
		b := data[i]
		switch {
		case i%2 == 0 && i+1 < len(data):
			b = data[i+1]
		case i%2 == 1:
			b = data[i-1]
		}
		crc = (crc << 8) ^ crc32_0x04C11DB7[byte(crc>>24)^b]
	}

	return crc
}
