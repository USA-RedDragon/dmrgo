// Package crc implements the CRCs and the checksum of ETSI TS 102 361-1
// B.3.7 to B.3.13: CRC-7, CRC-8, CRC-9, CRC-CCITT, CRC-32 and the 5-bit
// checksum of the embedded LC. For each, crc.<Name>(data) computes the value
// and crc.Check<Name>(data, want) compares it with a received one. The Data
// Type CRC Masks of Table B.21 are not applied here; see pdu.CRCMask.
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
