package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
)

// AddressHash returns the 8-bit hash of a 24-bit address, as in the HashTS1
// and HashTS2 fields of the Activity Update Short LC (ETSI TS 102 361-2
// clause 7.1.3.2) and the Hashed Source Address (ETSI TS 102 361-4 Table 6.79).
// It is the CRC-8 of the 24 address bits, MSB first (ETSI TS 102 361-1
// clause B.3.7). It returns a *dmrgo.FieldRangeError when id does not fit
// in 24 bits.
func AddressHash(id uint32) (uint8, error) {
	if id>>24 != 0 {
		return 0, errAddressHashRange
	}
	var b [3]byte
	putUint24(b[:], id)
	return crc.CRC8(b[:], 24), nil
}

// errAddressHashRange is the error of an AddressHash whose id does not fit
// in 24 bits.
var errAddressHashRange = &dmrgo.FieldRangeError{Type: "pdu", Field: "AddressHash", Bits: 24}
