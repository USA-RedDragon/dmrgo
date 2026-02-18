package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/dmr/fec"
)

// DecodeGolay24128 decodes a 24-bit word using the Golay (24,12,8) code.
// It returns the corrected 12-bit data and FECResult.
func DecodeGolay24128(received uint32) (data uint16, result fec.FECResult) {
	result.BitsChecked = 24
	minDist := 24
	bestData := uint16(0)

	// Brute-force search the lookup table for the nearest valid codeword.
	// Since there are only 4096 codewords, this is computationally feasible.
	for d, codeword := range Golay_24_12_8_table {
		dist := bits.OnesCount32(received ^ codeword)
		if dist < minDist {
			minDist = dist
			bestData = uint16(d) //nolint:gosec // d is a table index (0-4095), fits in uint16

			// If perfect match, we can stop early
			if dist == 0 {
				return bestData, result
			}
		}
	}

	// Golay (24,12,8) can correct up to 3 errors (d_min = 8, t = floor((8-1)/2) = 3)
	// Some sources suggest it can correct 3 errors and detect 4.
	if minDist <= 3 {
		result.ErrorsCorrected = minDist
		return bestData, result
	}

	// If nearest neighbor is too far, it's uncorrectable (or we found a false positive, but we report fail)
	// We return the best guess anyway, but flag it.
	result.ErrorsCorrected = minDist
	result.Uncorrectable = true
	return bestData, result
}
