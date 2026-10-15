package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
)

// AddressHash returns the 8-bit hashed form of a 24-bit address, as in the
// HashTS1 and HashTS2 fields of the Activity Update Short LC (ETSI TS 102
// 361-2 §7.1.3, note 1) and the Hashed Source Address of TS 102 361-4
// Table 6.79. It is the CRC-8 of ETSI TS 102 361-1 §B.3.7 over the 24
// address bits, MSB first. Bits above bit 23 of id are ignored.
func AddressHash(id uint32) uint8 {
	return crc.CalculateCRC8(bit.BitsFromUint32(id&0xFFFFFF, 24))
}
