package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
)

// AddressHash returns the 8-bit hash of a 24-bit address, as in the HashTS1
// and HashTS2 fields of the Activity Update Short LC (ETSI TS 102 361-2
// clause 7.1.3.2) and the Hashed Source Address (ETSI TS 102 361-4 Table 6.79).
// It is the CRC-8 of the 24 address bits, MSB first (ETSI TS 102 361-1
// clause B.3.7). Bits above bit 23 of id are ignored.
func AddressHash(id uint32) uint8 {
	var bits [24]bit.Bit
	bit.PutUint(bits[:], uint64(id))
	return crc.CRC8(bits[:])
}
