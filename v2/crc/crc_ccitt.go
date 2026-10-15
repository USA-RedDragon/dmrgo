package crc

// TS 102 361-1 V1.2.1 B.3.8 - CRC-CCITT calculation

// CCITT computes the CRC-CCITT over the given byte data
// using the table-based approach matching MMDVM's CCRC::checkCCITT162.
func CCITT(data []byte) uint16 {
	var crc16 uint16

	for _, b := range data {
		crc16 = (uint16(byte(crc16)) << 8) ^ ccitt16_0x1021[byte(crc16>>8)^b]
	}

	return ^crc16
}

// CheckCCITT reports whether want is the CRC-CCITT of data.
func CheckCCITT(data []byte, want uint16) bool {
	return CCITT(data) == want
}
