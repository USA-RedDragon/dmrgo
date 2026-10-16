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

// Encode returns PRFillBits and a nil error.
func (p *PRFill) Encode() ([96]bit.Bit, error) {
	return prFillBits, nil
}

// String returns a string representation of the PR FILL PDU.
func (p *PRFill) String() string {
	return "PR FILL (Idle)"
}

// IsPRFill checks whether the given 96 information bits match the
// PR FILL constant from Table D.2. It reads only the low bit of each value.
func IsPRFill(infoBits [96]bit.Bit) bool {
	for i, b := range infoBits {
		if b&1 != prFillBits[i] {
			return false
		}
	}
	return true
}
