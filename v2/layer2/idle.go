package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// TS 102 361-1 Annex D — Idle and Null message bit definitions

// IdleMessageInfoBits contains the 96 information bits of the idle message
// from Table D.2 of ETSI TS 102 361-1. It is the same value as pdu.PRFillBits.
//
//nolint:gochecknoglobals
var IdleMessageInfoBits = pdu.PRFillBits

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
