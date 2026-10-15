package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// IdleMessageInfo returns a copy of the 96 information bits of the idle
// message, the same value as pdu.PRFillBits (ETSI TS 102 361-1 Table D.2).
func IdleMessageInfo() [96]bit.Bit {
	return pdu.PRFillBits()
}

// IsIdleMessage reports whether infoBits is the idle message, as
// pdu.IsPRFill does (ETSI TS 102 361-1 Table D.2).
func IsIdleMessage(infoBits [96]bit.Bit) bool {
	return pdu.IsPRFill(infoBits)
}

// IsNullEmbeddedLC reports whether the 32 transmit bits are the Null
// embedded message, which is all zero (ETSI TS 102 361-1 Table D.1).
func IsNullEmbeddedLC(bits [32]bit.Bit) bool {
	lowbit.Mask(bits[:])
	return bits == [32]bit.Bit{}
}
