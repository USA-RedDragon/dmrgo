package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
)

// cachDeinterleaveTable maps transmit bit index (0 = first sent) to logical
// bit position. TACT bits are at transmit indices 0, 4, 8, 12, 14, 18, 22.
//
//nolint:gochecknoglobals
var cachDeinterleaveTable = [CACHBits]int{
	0, 7, 8, 9, 1, 10, 11, 12, 2, 13, 14, 15, 3, 16, 4, 17, 18, 19, 5, 20, 21, 22, 6, 23,
}

// cachInterleaveTable maps logical bit position to transmit bit index.
// This is the inverse of cachDeinterleaveTable.
//
//nolint:gochecknoglobals
var cachInterleaveTable = [CACHBits]int{
	0, 4, 8, 12, 14, 18, 22, 1, 2, 3, 5, 6, 7, 9, 10, 11, 13, 15, 16, 17, 19, 20, 21, 23,
}

// CACHDeinterleave converts a 24-bit CACH field from transmit order to the
// logical order that DecodeCACH takes: AT, TC, LCSS, Hamming parity, then
// the 17 payload bits (ETSI TS 102 361-1 clause B.4.1, Figure B.9).
func CACHDeinterleave(txBits [CACHBits]bit.Bit) [CACHBits]bit.Bit {
	lowbit.Mask(txBits[:])
	var logical [CACHBits]bit.Bit
	for i := 0; i < CACHBits; i++ {
		logical[cachDeinterleaveTable[i]] = txBits[i]
	}
	return logical
}

// CACHInterleave converts a 24-bit CACH field from logical order to
// transmit order. It is the inverse of CACHDeinterleave.
func CACHInterleave(logical [CACHBits]bit.Bit) [CACHBits]bit.Bit {
	lowbit.Mask(logical[:])
	var txBits [CACHBits]bit.Bit
	for i := 0; i < CACHBits; i++ {
		txBits[cachInterleaveTable[i]] = logical[i]
	}
	return txBits
}
