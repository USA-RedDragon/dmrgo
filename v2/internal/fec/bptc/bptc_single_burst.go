package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
)

// ETSI TS 102 361-1 §B.2.2 — Single Burst Variable Length BPTC
//
// The single-burst FEC uses a 2×16 encode matrix:
//   - Row 0: 11 info bits + 5 Hamming(16,11,4) parity bits
//   - Row 1: column parity (even for non-RC, odd for RC)
//
// Interleaving formula: Interleave Index = Index × 17 mod 32 (formula B.2)
//
// The interleaved 32-bit result is placed in the embedded signalling data
// field of a single voice burst.

// DecodeSingleBurst decodes 32 transmit bits from a single burst's
// embedded data field. Returns the 11 information bits and FEC result.
//
// Set oddParity=true for Reverse Channel (RC) bursts, where the column
// parity is odd (complemented) instead of even.
//
// Up to 3 bit errors in the 32 bits are corrected; ErrorsCorrected is the
// number of corrected bits. Four bit errors are always detected and set
// Uncorrectable.
func DecodeSingleBurst(txBits [32]bit.Bit, oddParity bool) ([11]bit.Bit, fec.Result) {
	lowbit.Mask(txBits[:])
	result := fec.Result{BitsChecked: 32}

	// Deinterleave: transmit order → encode matrix order
	var deinterleaved [32]bit.Bit
	for i := 0; i < 32; i++ {
		deinterleaved[single_burst_bptc_interleave[i]] = txBits[i]
	}

	// Unpack into 2×16 matrix
	// Encode matrix is read top-to-bottom, left-to-right:
	// Index 0 = matrix[0][0], Index 1 = matrix[1][0],
	// Index 2 = matrix[0][1], Index 3 = matrix[1][1], etc.
	var matrix [2][16]bit.Bit
	for i := 0; i < 32; i++ {
		row := i % 2
		col := i / 2
		matrix[row][col] = deinterleaved[i]
	}

	// Row 1 is a copy of row 0 (B.2.2.1) or its complement (B.2.2.2), so the
	// burst carries two copies of one Hamming(16,11,4) codeword and the pair
	// has minimum distance 8. Any codeword within distance 3 of the pair is
	// within distance 1 of one copy, so single-error decoding of each copy
	// finds every candidate.
	row1 := matrix[1]
	if oddParity {
		for col := range row1 {
			row1[col] ^= 1
		}
	}

	best, bestDist := matrix[0], 33
	for _, row := range [2][16]bit.Bit{matrix[0], row1} {
		candidate, ok := correctHamming16_11(row)
		if !ok {
			continue
		}
		if d := hammingDistance16(candidate, matrix[0]) + hammingDistance16(candidate, row1); d < bestDist {
			best, bestDist = candidate, d
		}
	}

	if bestDist > 3 {
		result.Uncorrectable = true
	} else {
		matrix[0] = best
		result.ErrorsCorrected = bestDist
	}

	// Extract 11 info bits from row 0, columns 0..10
	var info [11]bit.Bit
	copy(info[:], matrix[0][:11])

	return info, result
}

// EncodeSingleBurst encodes 11 information bits into 32 transmit bits
// for a single burst's embedded data field.
//
// Set oddParity=true for Reverse Channel (RC) encoding.
func EncodeSingleBurst(info [11]bit.Bit, oddParity bool) [32]bit.Bit {
	lowbit.Mask(info[:])
	var matrix [2][16]bit.Bit

	// Place info bits in row 0, columns 0..10
	copy(matrix[0][:11], info[:])

	// Compute Hamming(16,11,4) parity for row 0
	parity := parityHamming16_11(info)
	matrix[0][11] = parity[0]
	matrix[0][12] = parity[1]
	matrix[0][13] = parity[2]
	matrix[0][14] = parity[3]
	matrix[0][15] = parity[4]

	// Compute column parity row
	for col := 0; col < 16; col++ {
		matrix[1][col] = matrix[0][col]
		if oddParity {
			matrix[1][col] ^= 1 // complement for RC
		}
	}

	// Pack into linear stream (top-to-bottom, left-to-right)
	var stream [32]bit.Bit
	for col := 0; col < 16; col++ {
		stream[col*2] = matrix[0][col]
		stream[col*2+1] = matrix[1][col]
	}

	// Interleave: encode matrix order → transmit order
	var txBits [32]bit.Bit
	for i := 0; i < 32; i++ {
		txBits[(i*17)%32] = stream[i]
	}

	return txBits
}

// correctHamming16_11 corrects up to one bit error in a Hamming(16,11,4)
// codeword. ok is false when the syndrome does not match a single-bit error.
func correctHamming16_11(row [16]bit.Bit) ([16]bit.Bit, bool) {
	s := calculateSyndrome16_11(row)
	if s == 0 {
		return row, true
	}
	errPos := hamming16_11_syndrome_table[s]
	if errPos < 0 {
		return row, false
	}
	row[errPos] ^= 1
	return row, true
}

func hammingDistance16(a, b [16]bit.Bit) int {
	d := 0
	for i := range a {
		d += int(a[i] ^ b[i])
	}
	return d
}
