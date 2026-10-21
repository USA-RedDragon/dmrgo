package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
)

// AddressHash returns the 8-bit hash of a 24-bit address, as in the HashTS1
// and HashTS2 fields of the Activity Update Short LC and the Hashed Source
// Address. It is the CRC-8 of the 24 address bits, MSB first. It returns a
// *dmrgo.FieldRangeError when id does not fit in 24 bits.
// (ETSI TS 102 361-2 clause 7.1.3.2, ETSI TS 102 361-4 V1.12.1 Table 6.79,
// ETSI TS 102 361-1 clause B.3.7)
func AddressHash(id uint32) (uint8, error) {
	if id>>24 != 0 {
		return 0, errAddressHashRange
	}
	var b [3]byte
	putUint24(b[:], id)
	return crc.CRC8(b[:], 24), nil
}

var errAddressHashRange = &dmrgo.FieldRangeError{Type: "pdu", Field: "AddressHash", Bits: 24}
