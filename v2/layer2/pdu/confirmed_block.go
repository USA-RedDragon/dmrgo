package pdu

import "github.com/USA-RedDragon/dmrgo/v2/internal/crc"

// ETSI TS 102 361-1 Tables 9.11, 9.15; clause B.3.10

const confirmedMaxCRC9Octets = 22 + 1

func confirmedDBSN(block []byte) uint8 {
	return block[0] >> 1
}

func confirmedCRC9(block []byte) uint16 {
	return uint16(block[0]&1)<<8 | uint16(block[1])
}

// clause B.3.10
func computeConfirmedCRC9(dbsn uint8, userData []byte, mask uint16) uint16 {
	var buf [confirmedMaxCRC9Octets]byte
	n := copy(buf[:], userData)
	buf[n] = dbsn << 1
	return (crc.CRC9(buf[:], n*8+7) ^ mask) & 0x1FF
}

func checkConfirmedCRC9(block []byte, mask uint16) bool {
	return computeConfirmedCRC9(confirmedDBSN(block), block[2:], mask) == confirmedCRC9(block)
}

func setConfirmed(block []byte, dbsn uint8, userData []byte, mask uint16) {
	c := computeConfirmedCRC9(dbsn, userData, mask)
	block[0] = dbsn<<1 | byte((c>>8)&0x01)
	block[1] = byte(c & 0xFF)
	copy(block[2:], userData)
}
