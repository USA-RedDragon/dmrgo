package layer2

import "github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"

// IdleMessageInfo returns a copy of the 96 information bits of the idle
// message, packed, the same value as pdu.PRFillBits (ETSI TS 102 361-1 Table
// D.2).
func IdleMessageInfo() [12]byte {
	return pdu.PRFillBits()
}

// IsIdleMessage reports whether the 96 packed information bits info are the
// idle message, as pdu.IsPRFill does (ETSI TS 102 361-1 Table D.2).
func IsIdleMessage(info [12]byte) bool {
	return pdu.IsPRFill(info)
}

// IsNullEmbeddedLC reports whether the 32-bit embedded signalling field
// data, packed as RawBurst.EmbeddedData returns it, is the Null embedded
// message, which is all zero (ETSI TS 102 361-1 Table D.1).
func IsNullEmbeddedLC(data [4]byte) bool {
	return data == [4]byte{}
}
