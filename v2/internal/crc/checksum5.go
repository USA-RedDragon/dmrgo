package crc

// Checksum5 computes the 5-bit checksum of the 9 bytes of a 72-bit LC: their
// sum modulo 31, in the range 0 to 30 (ETSI TS 102 361-1 clause B.3.11).
func Checksum5(data [9]byte) uint8 {
	var sum uint16
	for _, b := range data {
		sum += uint16(b)
	}
	return uint8(sum % 31) //nolint:gosec // sum%31 always fits in uint8 (max 30)
}

// CheckChecksum5 reports whether want is the 5-bit checksum of data.
func CheckChecksum5(data [9]byte, want uint8) bool {
	return Checksum5(data) == want
}
