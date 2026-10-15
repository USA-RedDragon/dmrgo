package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// crc9MaskRate1 is the CRC-9 mask for Rate 1 Data Continuation
// per ETSI TS 102 361-1 Table B.21.
const crc9MaskRate1 uint16 = 0x10F

// ETSI TS 102 361-1 - 9.2.15 Rate 1 coded packet Data (R_1_DATA) PDU
//
// Data holds all 192 bits. Unconfirmed data (Table 9.18A) uses all of them
// as user data. Confirmed data (Table 9.18) carries a 7-bit DBSN, a 9-bit
// CRC-9 and 176 bits of user data; use the Confirmed* methods for it.
type Rate1Data struct {
	Data [24]byte `dmr:"bits:0-191,packed"`
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
// the masked CRC-9. Only the low 7 bits of dbsn are used.
func (s *Rate1Data) SetConfirmed(dbsn uint8, userData [22]byte) {
	setConfirmed(s.Data[:], dbsn, userData[:], crc9MaskRate1)
}
