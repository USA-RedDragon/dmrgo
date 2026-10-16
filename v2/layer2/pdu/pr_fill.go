package pdu

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// PRFillBits returns a copy of the 96 pseudo-random fill bits from Table D.2,
// packed, the first bit in the most significant bit of byte 0.
func PRFillBits() [12]byte {
	return prFill
}

// PRFill is the Pseudo Random Fill Bit (PR FILL) PDU of an idle burst, the
// 96 fixed bits of Table D.2 (ETSI TS 102 361-1 clause 9.1.9).
type PRFill struct{}

// DataType returns enums.DataTypeIdle.
func (p *PRFill) DataType() enums.DataType {
	return enums.DataTypeIdle
}

// Encode returns PRFillBits and a nil error.
func (p *PRFill) Encode() ([12]byte, error) {
	return prFill, nil
}

// String returns a string representation of the PR FILL PDU.
func (p *PRFill) String() string {
	return "PR FILL (Idle)"
}

// IsPRFill reports whether the 96 information bits info, packed as
// PRFillBits returns them, are the PR FILL bits of Table D.2.
func IsPRFill(info [12]byte) bool {
	return info == prFill
}
