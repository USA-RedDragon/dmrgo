package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// CRC9MaskRate12 is the CRC-9 mask for Rate 1/2 Data Continuation
// per ETSI TS 102 361-1 Table B.21.
const CRC9MaskRate12 uint16 = 0x0F0

// ETSI TS 102 361-1 - 9.2.7 Rate 1/2 coded packet Data (R_1_2_DATA) PDU
//
// Data holds all 96 bits. Unconfirmed data (Table 9.15AA) uses all of them
// as user data. Confirmed data (Table 9.15A) carries a 7-bit DBSN, a 9-bit
// CRC-9 and 80 bits of user data; use the Confirmed* methods for it.
type Rate12Data struct {
	DataType elements.DataType `dmr:"-"`
	Data     [12]byte          `dmr:"bits:0-95,packed"`
}

func (rtData *Rate12Data) GetDataType() elements.DataType {
	return rtData.DataType
}

// ConfirmedDBSN returns the Data Block Serial Number of a confirmed block.
func (rtData *Rate12Data) ConfirmedDBSN() uint8 {
	return confirmedDBSN(rtData.Data[:])
}

// ConfirmedCRC9 returns the received (masked) CRC-9 of a confirmed block.
func (rtData *Rate12Data) ConfirmedCRC9() uint16 {
	return confirmedCRC9(rtData.Data[:])
}

// ConfirmedUserData returns the 10 user data octets of a confirmed block.
// In a last block (R_1_2_LDATA, Table 9.15B) the last 4 octets are the
// 32-bit message CRC.
func (rtData *Rate12Data) ConfirmedUserData() [10]byte {
	var out [10]byte
	copy(out[:], rtData.Data[2:])
	return out
}

// CheckConfirmedCRC9 reports whether the CRC-9 of a confirmed block is valid.
func (rtData *Rate12Data) CheckConfirmedCRC9() bool {
	return checkConfirmedCRC9(rtData.Data[:], CRC9MaskRate12)
}

// SetConfirmed fills Data as a confirmed block (Table 9.15A) and computes
// the masked CRC-9. Only the low 7 bits of dbsn are used.
func (rtData *Rate12Data) SetConfirmed(dbsn uint8, userData [10]byte) {
	setConfirmed(rtData.Data[:], dbsn, userData[:], CRC9MaskRate12)
}
