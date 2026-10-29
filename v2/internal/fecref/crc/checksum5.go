package crc

// Checksum5 computes the 5-bit checksum of the 9 bytes of a 72-bit LC: their
// sum modulo 31, in the range 0 to 30 (ETSI TS 102 361-1 clause B.3.11).
func Checksum5(data [9]byte) uint8 {
	var sum uint16
	for _, b := range data {
		sum += uint16(b)
	}
	return uint8(sum % 31)
}
