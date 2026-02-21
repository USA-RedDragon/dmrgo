package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// TS 102 361-1 Annex D — Idle and Null message bit definitions

// IdleMessageInfoBits contains the 96 information bits of the idle message
// from Table D.2 of ETSI TS 102 361-1.
//
// In packed hex (MSB-first): FF 83 DF 16 64 21 4E 16 EC 5C CD 49
//
//nolint:gochecknoglobals
var IdleMessageInfoBits = func() [96]bit.Bit {
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

// NullEmbeddedLCBits contains the 32 transmit bits of the null embedded LC
// message from Table D.1 of ETSI TS 102 361-1.
//
// All 32 bits are zero — when the 11 information bits of the embedded message
// are all 0, all FEC and parity check bits are also 0.
//
//nolint:gochecknoglobals
var NullEmbeddedLCBits = [32]bit.Bit{}

// IdleMessageFECMatrix contains the 13×15 BPTC(196,96) encoded idle message
// from Figure D.1 of ETSI TS 102 361-1. This is the full 196-bit FEC-encoded
// representation of the idle message information bits.
//
// Layout: idleMessageFECMatrix[row][col], 13 rows × 15 columns, row-major.
//
//nolint:gochecknoglobals
var IdleMessageFECMatrix = [13][15]bit.Bit{
	{0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 0},
	{1, 0, 0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 1},
	{1, 1, 1, 1, 1, 0, 0, 0, 1, 0, 1, 1, 1, 0, 1},
	{1, 1, 0, 0, 1, 1, 0, 0, 1, 0, 0, 0, 1, 0, 1},
	{0, 0, 0, 1, 0, 0, 1, 0, 1, 0, 0, 0, 1, 1, 1},
	{1, 1, 1, 0, 1, 1, 0, 1, 0, 0, 0, 1, 1, 0, 1},
	{1, 1, 1, 1, 0, 0, 1, 1, 1, 1, 1, 0, 0, 1, 0},
	{0, 0, 1, 1, 0, 1, 1, 0, 0, 0, 1, 1, 1, 0, 1},
	{0, 1, 0, 1, 0, 0, 1, 0, 0, 0, 1, 0, 1, 0, 1},
	{0, 1, 0, 0, 1, 1, 1, 0, 0, 1, 0, 0, 0, 1, 1},
	{1, 0, 1, 1, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 0},
	{0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0},
	{0, 1, 0, 0, 1, 0, 1, 0, 1, 1, 1, 0, 1, 1, 0},
}

// IsIdleMessage checks whether the given 96 information bits match the
// idle message pattern defined in ETSI TS 102 361-1 Annex D, Table D.2.
func IsIdleMessage(infoBits [96]bit.Bit) bool {
	return infoBits == IdleMessageInfoBits
}

// IsNullEmbeddedLC checks whether the given 32 transmit bits match the
// null embedded LC message (all zeros) defined in ETSI TS 102 361-1
// Annex D, Table D.1.
func IsNullEmbeddedLC(bits [32]bit.Bit) bool {
	return bits == NullEmbeddedLCBits
}
