package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// crc9MaskRate1 is the CRC-9 mask for Rate 1 Data Continuation
// per ETSI TS 102 361-1 Table B.21.
const crc9MaskRate1 uint16 = 0x10F

// Rate1Data is a rate 1 coded packet data block (R_1_DATA) (ETSI TS 102 361-1
// clause 9.2.15). Unconfirmed data uses all 192 bits as user data. The
// Confirmed methods read the 7-bit DBSN, the 9-bit CRC-9 and the 176 user
// data bits of a confirmed block (Table 9.18), and SetConfirmed writes them.
type Rate1Data struct {
	Data [24]byte `dmr:"bits:0-191,packed"` // the 192 block bits, most significant bit first
}

// DataType returns enums.DataTypeRate1.
func (s *Rate1Data) DataType() enums.DataType {
	return enums.DataTypeRate1
}

// ConfirmedDBSN returns the Data Block Serial Number of a confirmed block.
func (s *Rate1Data) ConfirmedDBSN() uint8 {
	return confirmedDBSN(s.Data[:])
}

// ConfirmedCRC9 returns the received (masked) CRC-9 of a confirmed block.
func (s *Rate1Data) ConfirmedCRC9() uint16 {
	return confirmedCRC9(s.Data[:])
}

// ConfirmedUserData returns the 22 user data octets of a confirmed block.
// In a last block (R_1_LDATA, Table 9.18B) the last 4 octets are the
// 32-bit message CRC.
func (s *Rate1Data) ConfirmedUserData() [22]byte {
	var out [22]byte
	copy(out[:], s.Data[2:])
	return out
}

// ConfirmedCRC9OK reports whether the CRC-9 of a confirmed block is valid.
func (s *Rate1Data) ConfirmedCRC9OK() bool {
	return checkConfirmedCRC9(s.Data[:], crc9MaskRate1)
}

// SetConfirmed fills Data as a confirmed block (Table 9.18) and computes
// the masked CRC-9. It returns a *dmrgo.FieldRangeError, and changes
// nothing, when dbsn does not fit in 7 bits.
func (s *Rate1Data) SetConfirmed(dbsn uint8, userData [22]byte) error {
	if dbsn>>7 != 0 {
		return errRate1DataDBSNRange
	}
	setConfirmed(s.Data[:], dbsn, userData[:], crc9MaskRate1)
	return nil
}

// errRate1DataDBSNRange is the error of a SetConfirmed whose DBSN does
// not fit in 7 bits.
var errRate1DataDBSNRange = &dmrgo.FieldRangeError{Type: "Rate1Data", Field: fieldDBSN, Bits: 7}
