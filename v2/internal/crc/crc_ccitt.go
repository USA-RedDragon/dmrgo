// Package crc computes and checks the CRC-7, CRC-8, CRC-9, CRC-CCITT and
// CRC-32 values and the 5-bit embedded LC checksum of DMR (ETSI TS 102 361-1
// clauses B.3.7 to B.3.13). For each, crc.<Name>(data) computes the value and
// crc.Check<Name>(data, want) compares it with a received one. The package
// does not apply the Data Type CRC Masks (ETSI TS 102 361-1 Table B.21);
// pdu.CRCMask does.
package crc

// CCITT computes the inverted CRC-CCITT of data, as MMDVM's
// CCRC::checkCCITT162 does (ETSI TS 102 361-1 clause B.3.8).
func CCITT(data []byte) uint16 {
	return ^CCITTUpdate(0, data)
}

// CCITTUpdate returns the CRC-CCITT register after data, from the register
// reg, before the inversion. CCITT(data) is ^CCITTUpdate(0, data), so a CRC
// over several slices is ^CCITTUpdate(CCITTUpdate(0, a), b).
func CCITTUpdate(reg uint16, data []byte) uint16 {
	for _, b := range data {
		reg = (reg&0xFF)<<8 ^ ccitt16_0x1021[uint8(reg>>8)^b]
	}
	return reg
}

// CheckCCITT reports whether want is the CRC-CCITT of data.
func CheckCCITT(data []byte, want uint16) bool {
	return CCITT(data) == want
}
