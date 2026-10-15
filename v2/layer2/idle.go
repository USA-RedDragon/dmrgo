package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// TS 102 361-1 Annex D: Idle and Null message bit definitions

// IdleMessageInfo returns a copy of the 96 information bits of the idle
// message from Table D.2 of ETSI TS 102 361-1. It is the same value as
// pdu.PRFillBits. bptc.Encode196 of these bits gives the BPTC(196,96) idle
// message of Figure D.1.
func IdleMessageInfo() [96]bit.Bit {
	return pdu.PRFillBits()
}

// IsIdleMessage checks whether the given 96 information bits match the
// idle message pattern defined in ETSI TS 102 361-1 Annex D, Table D.2.
// It gives the same answer as pdu.IsPRFill.
func IsIdleMessage(infoBits [96]bit.Bit) bool {
	return pdu.IsPRFill(infoBits)
}

// IsNullEmbeddedLC checks whether the given 32 transmit bits match the
// null embedded LC message defined in ETSI TS 102 361-1 Annex D, Table D.1.
// All 32 bits are zero: when the 11 information bits of the embedded message
// are all 0, all FEC and parity check bits are also 0.
func IsNullEmbeddedLC(bits [32]bit.Bit) bool {
	lowbit.Mask(bits[:])
	return bits == [32]bit.Bit{}
}
