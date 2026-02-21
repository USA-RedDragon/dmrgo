package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// ETSI TS 102 361-1 - 9.1.9 Pseudo Random Fill Bit (PR FILL) PDU
//
// The PR FILL PDU is 96 bits of pre-defined pseudo-random data used for
// idle messages (see §7.3 and Annex D). The exact bit values are specified
// in Table D.2 — there is no runtime LFSR calculation. Implementations
// use the literal constant from the table.
//
// In packed hex (MSB-first): FF 83 DF 16 64 21 4E 16 EC 5C CD 49

// PRFillBits contains the 96 pseudo-random fill bits from Table D.2.
//
//nolint:gochecknoglobals
var PRFillBits = func() [96]bit.Bit {
	packed := [12]byte{
		0xFF, 0x83, 0xDF, 0x16, 0x64, 0x21,
		0x4E, 0x16, 0xEC, 0x5C, 0xCD, 0x49,
	}
	var bits [96]bit.Bit
	for i := 0; i < 96; i++ {
		if (packed[i/8]>>(7-(i%8)))&1 != 0 { //nolint:gosec // i/8 bounded: i < 96, so i/8 <= 11 < 12
			bits[i] = 1
		}
	}
	return bits
}()

// PRFill represents the PR FILL PDU (§9.1.9).
type PRFill struct {
	DataType elements.DataType
}

// GetDataType returns the data type of the PDU.
func (p *PRFill) GetDataType() elements.DataType {
	return p.DataType
}

// ToString returns a string representation of the PR FILL PDU.
func (p *PRFill) ToString() string {
	return "PR FILL (Idle)"
}

// IsPRFill checks whether the given 96 information bits match the
// PR FILL constant from Table D.2.
func IsPRFill(infoBits [96]bit.Bit) bool {
	return infoBits == PRFillBits
}
