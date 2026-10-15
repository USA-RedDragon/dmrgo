package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
)

// Confirmed data blocks (ETSI TS 102 361-1 Tables 9.11, 9.15A, 9.18) start
// with a 7-bit DBSN in octet 0 bits 7-1, then the CRC-9 with its MSB in
// octet 0 bit 0 and the rest in octet 1 (clause B.3.10), then user data.

func confirmedDBSN(block []byte) uint8 {
	return block[0] >> 1
}

func confirmedCRC9(block []byte) uint16 {
	return uint16(block[0]&1)<<8 | uint16(block[1])
}

// computeConfirmedCRC9 computes the masked CRC-9 over the user data followed
// by the 7 DBSN bits, which is the M(x) order given in clause B.3.10.
func computeConfirmedCRC9(dbsn uint8, userData []byte, mask uint16) uint16 {
	bits := make([]bit.Bit, len(userData)*8+7)
	copy(bits, bit.Unpack(userData))
	bit.PutUint(bits[len(bits)-7:], uint64(dbsn))
	return (crc.CalculateCRC9(bits) ^ mask) & 0x1FF
}

func checkConfirmedCRC9(block []byte, mask uint16) bool {
	return computeConfirmedCRC9(confirmedDBSN(block), block[2:], mask) == confirmedCRC9(block)
}

func setConfirmed(block []byte, dbsn uint8, userData []byte, mask uint16) {
	c := computeConfirmedCRC9(dbsn, userData, mask)
	block[0] = (dbsn&0x7F)<<1 | byte((c>>8)&0x01)
	block[1] = byte(c & 0xFF)
	copy(block[2:], userData)
}
