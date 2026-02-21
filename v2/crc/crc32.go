package crc

// TS 102 361-1 - B.3.9  32-bit CRC calculation
//
// Generator polynomial:
//   GM(x) = x³² + x²⁶ + x²³ + x²² + x¹⁶ + x¹² + x¹¹ + x¹⁰ + x⁸ + x⁷ + x⁵ + x⁴ + x² + x + 1
//   (0x04C11DB7)
//
// Initial remainder: 0x00000000
// Final XOR: none
//
// FM(x) = x³² · M(x) mod GM(x)
//
// The input data bytes are organized in 16-bit words, and within each word
// the bytes are swapped (LSO before MSO) before CRC computation.

// CalculateCRC32 computes the 32-bit CRC over the given byte data.
// The data bytes are swapped within 16-bit words per the DMR spec before
// polynomial division. Uses the generated crc32_0x04C11DB7 lookup table
// for byte-at-a-time processing.
func CalculateCRC32(data []byte) uint32 {
	// Swap bytes within 16-bit words
	swapped := make([]byte, len(data))
	for i := 0; i+1 < len(data); i += 2 {
		swapped[i] = data[i+1]
		swapped[i+1] = data[i]
	}
	// If odd number of bytes, last byte stays as-is
	if len(data)%2 == 1 {
		swapped[len(data)-1] = data[len(data)-1]
	}

	var crc uint32
	for _, b := range swapped {
		crc = (crc << 8) ^ crc32_0x04C11DB7[byte(crc>>24)^b]
	}

	return crc
}

// CheckCRC32 validates a 32-bit CRC. The expected CRC is compared against the
// computed CRC of the data. The CRC bytes in the data are stored in
// little-endian order per the DMR spec.
func CheckCRC32(data []byte, expected uint32) bool {
	return CalculateCRC32(data) == expected
}
