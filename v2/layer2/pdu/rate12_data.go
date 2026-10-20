package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ETSI TS 102 361-1 Table B.21
const crc9MaskRate12 uint16 = 0x0F0

// Rate12Data is a rate 1/2 coded packet data block (R_1_2_DATA). Unconfirmed
// data uses all 96 bits as user data. The Confirmed methods read the 7-bit
// DBSN, the 9-bit CRC-9 and the 80 user data bits of a confirmed block, and
// SetConfirmed writes them. (ETSI TS 102 361-1 clause 9.2.7, Table 9.15A)
type Rate12Data struct {
	Data [12]byte `dmr:"bits:0-95,packed"` // the 96 block bits, most significant bit first
}

// DataType returns enums.DataTypeRate12.
func (s *Rate12Data) DataType() enums.DataType {
	return enums.DataTypeRate12
}

// ConfirmedDBSN returns the Data Block Serial Number of a confirmed block.
func (s *Rate12Data) ConfirmedDBSN() uint8 {
	return confirmedDBSN(s.Data[:])
}

// ConfirmedCRC9 returns the received (masked) CRC-9 of a confirmed block.
func (s *Rate12Data) ConfirmedCRC9() uint16 {
	return confirmedCRC9(s.Data[:])
}

// ConfirmedUserData returns the 10 user data octets of a confirmed block.
// In a last block (R_1_2_LDATA, Table 9.15B) the last 4 octets are the
// 32-bit message CRC.
func (s *Rate12Data) ConfirmedUserData() [10]byte {
	var out [10]byte
	copy(out[:], s.Data[2:])
	return out
}

// ConfirmedCRC9OK reports whether the CRC-9 of a confirmed block is valid.
func (s *Rate12Data) ConfirmedCRC9OK() bool {
	return checkConfirmedCRC9(s.Data[:], crc9MaskRate12)
}

// SetConfirmed fills Data as a confirmed block (Table 9.15A) and computes
// the masked CRC-9. It returns a *dmrgo.FieldRangeError, and changes
// nothing, when dbsn does not fit in 7 bits.
func (s *Rate12Data) SetConfirmed(dbsn uint8, userData [10]byte) error {
	if dbsn>>7 != 0 {
		return errRate12DataDBSNRange
	}
	setConfirmed(s.Data[:], dbsn, userData[:], crc9MaskRate12)
	return nil
}

var errRate12DataDBSNRange = &dmrgo.FieldRangeError{Type: "Rate12Data", Field: fieldDBSN, Bits: 7}
