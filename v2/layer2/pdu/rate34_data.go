package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// crc9MaskRate34 is the CRC-9 mask for Rate 3/4 Data Continuation
// per ETSI TS 102 361-1 Table B.21.
const crc9MaskRate34 uint16 = 0x1FF

// Rate34Data is a rate 3/4 coded packet data block (R_3_4_DATA). Unconfirmed
// data uses all 144 bits as user data. The Confirmed methods read the 7-bit
// DBSN, the 9-bit CRC-9 and the 128 user data bits of a confirmed block, and
// SetConfirmed writes them. (ETSI TS 102 361-1 clause 9.2.2, Table 9.11)
type Rate34Data struct {
	Data [18]byte `dmr:"bits:0-143,packed"` // the 144 block bits, most significant bit first
}

// DataType returns enums.DataTypeRate34.
func (s *Rate34Data) DataType() enums.DataType {
	return enums.DataTypeRate34
}

// ConfirmedDBSN returns the Data Block Serial Number of a confirmed block.
func (s *Rate34Data) ConfirmedDBSN() uint8 {
	return confirmedDBSN(s.Data[:])
}

// ConfirmedCRC9 returns the received (masked) CRC-9 of a confirmed block.
func (s *Rate34Data) ConfirmedCRC9() uint16 {
	return confirmedCRC9(s.Data[:])
}

// ConfirmedUserData returns the 16 user data octets of a confirmed block.
// In a last block (R_3_4_LDATA, Table 9.12) the last 4 octets are the
// 32-bit message CRC.
func (s *Rate34Data) ConfirmedUserData() [16]byte {
	var out [16]byte
	copy(out[:], s.Data[2:])
	return out
}

// ConfirmedCRC9OK reports whether the CRC-9 of a confirmed block is valid.
func (s *Rate34Data) ConfirmedCRC9OK() bool {
	return checkConfirmedCRC9(s.Data[:], crc9MaskRate34)
}

// SetConfirmed fills Data as a confirmed block (Table 9.11) and computes
// the masked CRC-9. It returns a *dmrgo.FieldRangeError, and changes
// nothing, when dbsn does not fit in 7 bits.
func (s *Rate34Data) SetConfirmed(dbsn uint8, userData [16]byte) error {
	if dbsn>>7 != 0 {
		return errRate34DataDBSNRange
	}
	setConfirmed(s.Data[:], dbsn, userData[:], crc9MaskRate34)
	return nil
}

// errRate34DataDBSNRange is the error of a SetConfirmed whose DBSN does
// not fit in 7 bits.
var errRate34DataDBSNRange = &dmrgo.FieldRangeError{Type: "Rate34Data", Field: fieldDBSN, Bits: 7}
