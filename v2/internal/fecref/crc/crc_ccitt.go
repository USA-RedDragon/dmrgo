// Package crc computes the CRC-7, CRC-8, CRC-9, CRC-CCITT and CRC-32
// values and the 5-bit embedded LC checksum of DMR. For each,
// crc.<Name>(data) computes the value. For CRC-7, CRC-8 and CRC-9,
// crc.Check<Name>(bits, want) compares it with a received one. The package does not apply the Data Type
// CRC Masks; pdu.CRCMask does. (ETSI TS 102 361-1 clauses B.3.7 to B.3.13,
// Table B.21)
package crc

// CCITT computes the inverted CRC-CCITT of data, as MMDVM's
// CCRC::checkCCITT162 does (ETSI TS 102 361-1 clause B.3.8).
func CCITT(data []byte) uint16 {
	var crc16 uint16

	for _, b := range data {
		crc16 = (uint16(byte(crc16)) << 8) ^ ccitt16_0x1021[byte(crc16>>8)^b]
	}

	return ^crc16
}
