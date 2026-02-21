package crc

// TS 102 361-1 V1.2.1 B.3.8 - CRC-CCITT calculation

// CalculateCRCCCITT computes the CRC-CCITT over the given byte data
// using the table-based approach matching MMDVM's CCRC::checkCCITT162.
func CalculateCRCCCITT(data []byte) uint16 {
	var crc16 uint16

	for _, b := range data {
		crc16 = (uint16(byte(crc16)) << 8) ^ ccitt16_0x1021[byte(crc16>>8)^b]
	}

	return ^crc16
}

// CheckCRCCCITT validates the CRC-CCITT over the given byte data.
// The last two bytes of the data are the CRC (big-endian, swapped like MMDVM).
func CheckCRCCCITT(data []byte) bool {
	if len(data) < 3 {
		return false
	}

	crc := CalculateCRCCCITT(data[:len(data)-2])

	// MMDVM compares: crc8[0] == in[length-1] && crc8[1] == in[length-2]
	return byte(crc) == data[len(data)-1] && byte(crc>>8) == data[len(data)-2]
}
