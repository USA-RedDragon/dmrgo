package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ETSI TS 102 361-1 - 9.1.9 Pseudo Random Fill Bit (PR FILL) PDU
//
// The PR FILL PDU is 96 bits of pre-defined pseudo-random data used for
// idle messages (see §7.3 and Annex D). The exact bit values are specified
// in Table D.2 — there is no runtime LFSR calculation. Implementations
// use the literal constant from the table.
//
// In packed hex, I(95) first: FF 83 DF 17 32 09 4E D1 E7 CD 8A 91

//nolint:gochecknoglobals
var prFillBits = func() [96]bit.Bit {
	packed := [12]byte{
		0xFF, 0x83, 0xDF, 0x17, 0x32, 0x09,
		0x4E, 0xD1, 0xE7, 0xCD, 0x8A, 0x91,
	}
	var bits [96]bit.Bit
	for i := 0; i < 96; i++ {
		if (packed[i/8]>>(7-(i%8)))&1 != 0 { //nolint:gosec // i/8 bounded: i < 96, so i/8 <= 11 < 12
			bits[i] = 1
		}
	}
	return bits
}()

// PRFillBits returns a copy of the 96 pseudo-random fill bits from Table D.2.
func PRFillBits() [96]bit.Bit {
	return prFillBits
}

// PRFill represents the PR FILL PDU (§9.1.9).
type PRFill struct {
	DataType enums.DataType
}

// GetDataType returns the data type of the PDU.
func (p *PRFill) GetDataType() enums.DataType {
	return p.DataType
}

// ToString returns a string representation of the PR FILL PDU.
func (p *PRFill) ToString() string {
	return "PR FILL (Idle)"
}

// IsPRFill checks whether the given 96 information bits match the
// PR FILL constant from Table D.2.
func IsPRFill(infoBits [96]bit.Bit) bool {
	return infoBits == prFillBits
}
