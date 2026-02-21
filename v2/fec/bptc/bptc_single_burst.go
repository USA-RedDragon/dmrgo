package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
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

// DecodeSingleBurstBPTC decodes 32 transmit bits from a single burst's
// embedded data field. Returns the 11 information bits and FEC result.
//
// Set oddParity=true for Reverse Channel (RC) bursts, where the column
// parity is odd (complemented) instead of even.
func DecodeSingleBurstBPTC(txBits [32]bit.Bit, oddParity bool) ([11]bit.Bit, fec.FECResult) {
	result := fec.FECResult{BitsChecked: 32}

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

	// Column parity check
	for col := 0; col < 16; col++ {
		parity := matrix[0][col] ^ matrix[1][col]
		if oddParity {
			// RC: parity row is complement, so XOR should be 1
			if parity != 1 {
				// Column parity error — row Hamming should help
			}
		} else {
			// Non-RC: even parity, XOR should be 0
			if parity != 0 {
				// Column parity error
			}
		}
	}

	// Row Hamming(16,11,4) correction on row 0
	s := calculateSyndrome16_11(matrix[0])
	if s != 0 {
		errPos := hamming16_11_syndrome_table[s]
		if errPos >= 0 && errPos < 16 {
			matrix[0][errPos] ^= 1
			result.ErrorsCorrected++
		} else {
			result.Uncorrectable = true
		}
	}

	// Recompute expected parity row from corrected row 0
	if !result.Uncorrectable {
		for col := 0; col < 16; col++ {
			expected := matrix[0][col]
			if oddParity {
				expected ^= 1 // complement for RC
			}
			if matrix[1][col] != expected {
				// Parity row has error — but we can't correct it independently.
				// Since Hamming already corrected row 0, flag as detected.
				result.Uncorrectable = true
				break
			}
		}
	}

	// Extract 11 info bits from row 0, columns 0..10
	var info [11]bit.Bit
	copy(info[:], matrix[0][:11])

	return info, result
}

// EncodeSingleBurstBPTC encodes 11 information bits into 32 transmit bits
// for a single burst's embedded data field.
//
// Set oddParity=true for Reverse Channel (RC) encoding.
func EncodeSingleBurstBPTC(info [11]bit.Bit, oddParity bool) [32]bit.Bit {
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
