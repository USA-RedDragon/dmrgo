package layer2

import "github.com/USA-RedDragon/dmrgo/v2/bit"

// ETSI TS 102 361-1 §B.4.1 — CACH Burst Interleaving
//
// The 24-bit CACH burst is transmitted with timing, activity, and Hamming
// parity bits spread across the burst for fade resistance. The 7 TACT bits
// (AT, TC, LCSS, Hamming parity) are interleaved with 17 payload bits.
//
// Logical order:  [AT, TC, LS(1), LS(0), H(2), H(1), H(0), P(16)..P(0)]
// Transmit order: [AT, P(16), P(15), P(14), TC, P(13), ..., H(0), P(0)]
//
// See Figure B.9 in the spec. The figure numbers the transmitted bits
// TX(23) (sent first) down to TX(0) (sent last). This library indexes bits
// in the order sent, so TX(n) is index 23-n.

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

// CACHDeinterleave converts a 24-bit CACH field from over-the-air transmit
// order to logical order suitable for DecodeCACH.
//
// Input:  txBits in transmit order (Figure B.9 bottom row)
// Output: logical order (AT, TC, LCSS, Hamming parity, then 17 payload bits)
func CACHDeinterleave(txBits [CACHBits]bit.Bit) [CACHBits]bit.Bit {
	var logical [CACHBits]bit.Bit
	for i := 0; i < CACHBits; i++ {
		logical[cachDeinterleaveTable[i]] = txBits[i]
	}
	return logical
}

// CACHInterleave converts a 24-bit CACH field from logical order to
// over-the-air transmit order.
//
// Input:  logical order (AT, TC, LCSS, Hamming parity, then 17 payload bits)
// Output: txBits in transmit order (Figure B.9 bottom row)
func CACHInterleave(logical [CACHBits]bit.Bit) [CACHBits]bit.Bit {
	var txBits [CACHBits]bit.Bit
	for i := 0; i < CACHBits; i++ {
		txBits[cachInterleaveTable[i]] = logical[i]
	}
	return txBits
}
