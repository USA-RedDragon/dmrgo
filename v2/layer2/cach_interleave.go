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
// See Figure B.9 in the spec.

// cachDeinterleaveTable maps transmit bit position → logical bit position.
// Derived from Figure B.9: TX(23) → logical 0 (AT), TX(22) → logical 7 (P16), etc.
// TX bits are numbered MSB-first: TX(23) is sent first, TX(0) is sent last.
//
//nolint:gochecknoglobals
var cachDeinterleaveTable = [CACHBits]int{
	23, 6, 22, 21, 20, 5, 19, 18, 17, 4, 16, 3, 15, 14, 13, 2, 12, 11, 10, 1, 9, 8, 7, 0,
}

// cachInterleaveTable maps logical bit position → transmit bit position.
// This is the inverse of cachDeinterleaveTable.
//
//nolint:gochecknoglobals
var cachInterleaveTable = [CACHBits]int{
	23, 19, 15, 11, 9, 5, 1, 22, 21, 20, 18, 17, 16, 14, 13, 12, 10, 8, 7, 6, 4, 3, 2, 0,
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
