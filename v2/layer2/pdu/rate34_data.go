package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// CRC9MaskRate34 is the CRC-9 mask for Rate 3/4 Data Continuation
// per ETSI TS 102 361-1 Table B.21.
const CRC9MaskRate34 uint16 = 0x1FF

// ETSI TS 102 361-1 - 9.2.2 Rate 3/4 coded packet Data (R_3_4_DATA) PDU
//
// Data holds all 144 bits. Unconfirmed data (Table 9.11A) uses all of them
// as user data. Confirmed data (Table 9.11) carries a 7-bit DBSN, a 9-bit
// CRC-9 and 128 bits of user data; use the Confirmed* methods for it.
type Rate34Data struct {
	DataType elements.DataType `dmr:"-"`
	Data     [18]byte          `dmr:"bits:0-143,packed"`
}

func (rtData *Rate34Data) GetDataType() elements.DataType {
	return rtData.DataType
}

// ConfirmedDBSN returns the Data Block Serial Number of a confirmed block.
func (rtData *Rate34Data) ConfirmedDBSN() uint8 {
	return confirmedDBSN(rtData.Data[:])
}

// ConfirmedCRC9 returns the received (masked) CRC-9 of a confirmed block.
func (rtData *Rate34Data) ConfirmedCRC9() uint16 {
	return confirmedCRC9(rtData.Data[:])
}

// ConfirmedUserData returns the 16 user data octets of a confirmed block.
// In a last block (R_3_4_LDATA, Table 9.12) the last 4 octets are the
// 32-bit message CRC.
func (rtData *Rate34Data) ConfirmedUserData() [16]byte {
	var out [16]byte
	copy(out[:], rtData.Data[2:])
	return out
}

// CheckConfirmedCRC9 reports whether the CRC-9 of a confirmed block is valid.
func (rtData *Rate34Data) CheckConfirmedCRC9() bool {
	return checkConfirmedCRC9(rtData.Data[:], CRC9MaskRate34)
}

// SetConfirmed fills Data as a confirmed block (Table 9.11) and computes
// the masked CRC-9. Only the low 7 bits of dbsn are used.
func (rtData *Rate34Data) SetConfirmed(dbsn uint8, userData [16]byte) {
	setConfirmed(rtData.Data[:], dbsn, userData[:], CRC9MaskRate34)
}
