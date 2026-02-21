package crc

// TS 102 361-1 - B.3.11  5-bit Checksum (CS) calculation
//
// CS = [LC_0 + LC_1 + ... + LC_8] mod 31
//
// where LC_0 through LC_8 are the 9 octets of the 72-bit LC.
// The summation uses unsigned 16-bit arithmetic.
// The result is in the range [0, 30].

// CalculateChecksum5 computes the 5-bit checksum over 9 bytes of LC data.
// The result is the sum of all 9 bytes modulo 31 (range 0–30).
func CalculateChecksum5(data [9]byte) uint8 {
	var sum uint16
	for _, b := range data {
		sum += uint16(b)
	}
	return uint8(sum % 31) //nolint:gosec // sum%31 always fits in uint8 (max 30)
}

// CheckChecksum5 validates a 5-bit checksum against the expected value.
func CheckChecksum5(data [9]byte, expected uint8) bool {
	return CalculateChecksum5(data) == expected
}
