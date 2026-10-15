package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// PRFillBits returns a copy of the 96 pseudo-random fill bits from Table D.2.
func PRFillBits() [96]bit.Bit {
	return prFillBits
}

// PRFill is the Pseudo Random Fill Bit (PR FILL) PDU of an idle burst, the
// 96 fixed bits of Table D.2 (ETSI TS 102 361-1 clause 9.1.9).
type PRFill struct{}

// DataType returns enums.DataTypeIdle.
func (p *PRFill) DataType() enums.DataType {
	return enums.DataTypeIdle
}

// EncodeBits returns PRFillBits as a slice.
func (p *PRFill) EncodeBits() ([]bit.Bit, error) {
	bits := prFillBits
	return bits[:], nil
}

// String returns a string representation of the PR FILL PDU.
func (p *PRFill) String() string {
	return "PR FILL (Idle)"
}

// IsPRFill checks whether the given 96 information bits match the
// PR FILL constant from Table D.2.
func IsPRFill(infoBits [96]bit.Bit) bool {
	return infoBits == prFillBits
}
