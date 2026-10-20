// Package crc computes and checks the CRC-7, CRC-8, CRC-9, CRC-CCITT and
// CRC-32 values and the 5-bit embedded LC checksum of DMR. For each,
// crc.<Name>(data) computes the value and crc.Check<Name>(data, want)
// compares it with a received one. The package does not apply the Data Type
// CRC Masks; pdu.CRCMask does. (ETSI TS 102 361-1 clauses B.3.7 to B.3.13,
// Table B.21)
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
	t := ccittNibbles()
	for _, b := range data {
		reg = reg<<4 ^ t[reg>>12^uint16(b>>4)]
		reg = reg<<4 ^ t[reg>>12^uint16(b&0x0F)]
	}
	return reg
}

// ccittNibbles returns the CRC-CCITT register change of each 4-bit value
// shifted out, which takes 32 bytes where a byte table takes 512.
func ccittNibbles() [16]uint16 {
	return [16]uint16{
		0x0000, 0x1021, 0x2042, 0x3063, 0x4084, 0x50a5, 0x60c6, 0x70e7,
		0x8108, 0x9129, 0xa14a, 0xb16b, 0xc18c, 0xd1ad, 0xe1ce, 0xf1ef,
	}
}

// CheckCCITT reports whether want is the CRC-CCITT of data.
func CheckCCITT(data []byte, want uint16) bool {
	return CCITT(data) == want
}
